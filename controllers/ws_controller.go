package controllers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"flow-talk/middlewares"
	"flow-talk/models"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

const (
	// wsWriteWait 限制单次写入耗时，避免异常连接长期占用 goroutine。
	wsWriteWait = 10 * time.Second
	// wsReadLimit 限制单个事件大小。当前消息只保存 JSON 元数据，不接收二进制正文。
	wsReadLimit = 64 * 1024
	// wsReadTimeout 要大于前端 25-30 秒的应用层心跳间隔，用于回收断网后的僵尸连接。
	wsReadTimeout = 75 * time.Second
)

// WSController 处理 WebSocket 建连和事件分发。
type WSController struct {
	JWT              models.JWTConfig
	Hub              *models.WSHub
	Bus              models.RealtimeBus
	PresenceTracker  models.PresenceTracker
	PresenceProvider models.PresenceProvider
}

var wsUpgrader = websocket.Upgrader{
	// 当前项目暂未做浏览器来源白名单；生产环境应按前端域名收紧。
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Connect 处理 GET /api/ws?token={jwt}&device_id={device_id}。
// WebSocket 无法稳定携带 Authorization 头，所以这里直接解析 query token。
func (ctl WSController) Connect(c *gin.Context) {
	// WebSocket 建连先鉴权再升级协议。
	// 一旦 Upgrade 成功，后续响应就不再是普通 HTTP JSON，所以失败必须发生在 Upgrade 前。
	user, ok := ctl.userFromQueryToken(c)
	if !ok {
		return
	}

	socket, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}

	wsConn := models.NewWSConnection(user.ID, strings.TrimSpace(c.Query("device_id")))
	ctl.Hub.Add(wsConn)
	if err := ctl.presenceTracker().AddConnection(wsConn); err != nil {
		log.Printf("记录 WebSocket 上线状态失败: user_id=%d err=%v", user.ID, err)
	}
	// 设备表属于 v5 能力；这里忽略“不存在设备”的错误，让纯 WebSocket 调试不被设备上报流程阻塞。
	_ = models.TouchUserDevice(user.ID)
	ctl.publishCurrentPresence(user.ID)

	// 写循环独立 goroutine，从 wsConn.Send 队列取消息写回客户端。
	// 读循环留在当前请求 goroutine 中，直到连接断开后触发 defer 清理。
	go ctl.writeLoop(socket, wsConn)
	ctl.readLoop(user, socket, wsConn)
}

func (ctl WSController) userFromQueryToken(c *gin.Context) (models.User, bool) {
	// WebSocket 客户端不一定方便设置 Authorization header。
	// v4 约定 token 放在 query 中：/api/ws?token={jwt}&device_id={device_id}。
	token := strings.TrimSpace(c.Query("token"))
	if token == "" {
		c.AbortWithStatus(http.StatusUnauthorized)
		return models.User{}, false
	}

	claims, err := middlewares.VerifyToken(token, ctl.JWT.Secret)
	if err != nil {
		c.AbortWithStatus(http.StatusUnauthorized)
		return models.User{}, false
	}

	// token 只证明曾经登录过；建连时仍要查库确认用户没有被禁用。
	user, err := models.FindUserByID(claims.UserID)
	if err != nil || user.Status != models.UserStatusEnabled {
		c.AbortWithStatus(http.StatusUnauthorized)
		return models.User{}, false
	}
	return user, true
}

func (ctl WSController) readLoop(user models.User, socket *websocket.Conn, wsConn *models.WSConnection) {
	defer func() {
		// 任何读错误、客户端断开或服务端关闭都会走这里，确保 Hub 不保留脏连接。
		ctl.Hub.Remove(wsConn.UserID, wsConn.ID)
		if err := ctl.presenceTracker().RemoveConnection(wsConn); err != nil {
			log.Printf("记录 WebSocket 下线状态失败: user_id=%d err=%v", wsConn.UserID, err)
		}
		ctl.publishCurrentPresence(wsConn.UserID)
		_ = socket.Close()
	}()

	// 限制单个 JSON 事件大小，避免恶意客户端通过超大帧占用内存。
	socket.SetReadLimit(wsReadLimit)
	if err := socket.SetReadDeadline(time.Now().Add(wsReadTimeout)); err != nil {
		return
	}
	for {
		_, payload, err := socket.ReadMessage()
		if err != nil {
			return
		}
		if err := socket.SetReadDeadline(time.Now().Add(wsReadTimeout)); err != nil {
			return
		}

		ctl.Hub.Touch(wsConn.UserID, wsConn.ID)
		_ = ctl.presenceTracker().TouchConnection(wsConn)
		// 心跳或任意事件都可以视为设备活跃，用来支撑 v5 最近在线时间。
		_ = models.TouchUserDevice(wsConn.UserID)

		var event models.WSEvent
		if err := json.Unmarshal(payload, &event); err != nil {
			wsConn.SendEvent(models.NewWSErrorEvent("", "无效事件格式"))
			continue
		}
		ctl.handleEvent(user, event, wsConn)
	}
}

func (ctl WSController) writeLoop(socket *websocket.Conn, wsConn *models.WSConnection) {
	// 写失败时主动关闭 socket，让阻塞中的 ReadMessage 立即返回并执行统一下线清理。
	defer func() { _ = socket.Close() }()
	for payload := range wsConn.Send {
		// 每次写入都设置 deadline，防止客户端网络异常时 WriteMessage 永久阻塞。
		if err := socket.SetWriteDeadline(time.Now().Add(wsWriteWait)); err != nil {
			return
		}
		if err := socket.WriteMessage(websocket.TextMessage, payload); err != nil {
			return
		}
	}
}

func (ctl WSController) handleEvent(user models.User, event models.WSEvent, wsConn *models.WSConnection) {
	// 所有事件都先经过统一信封解析，再按 type 分发。
	// 新增 WebSocket 能力时优先扩展这里，而不是在 readLoop 里堆业务逻辑。
	switch event.Type {
	case models.WSEventPing:
		wsConn.SendEvent(models.NewPongEvent(event.RequestID))
	case models.WSEventMessageSend:
		ctl.handleMessageSend(user, event, wsConn)
	default:
		wsConn.SendEvent(models.NewWSErrorEvent(event.RequestID, "不支持的事件类型"))
	}
}

func (ctl WSController) handleMessageSend(user models.User, event models.WSEvent, wsConn *models.WSConnection) {
	var req models.WSMessageSendPayload
	if err := json.Unmarshal(event.Payload, &req); err != nil {
		wsConn.SendEvent(models.NewWSErrorEvent(event.RequestID, "参数校验失败"))
		return
	}

	// WebSocket 发送和 HTTP 发送共用 SendMessage。
	// 这样消息内容校验、成员权限、client_msg_id 幂等、最后消息更新都只有一套实现。
	result, err := models.SendMessageWithResult(user.ID, req.ConversationID, req.ClientMsgID, req.MessageType, req.Content)
	if err != nil {
		wsConn.SendEvent(models.NewWSErrorEvent(event.RequestID, wsMessageForError(err)))
		return
	}
	message := result.Message

	// ack 只回给当前连接，表示客户端这次发送请求已经完成入库。
	wsConn.SendEvent(models.NewWSEvent(models.WSEventMessageAck, event.RequestID, message))

	if !result.Created {
		return
	}

	// 只投递 active 成员；已退出或被移除成员不会收到实时消息。
	memberIDs, err := models.ListActiveConversationMemberIDs(message.ConversationID)
	if err != nil {
		wsConn.SendEvent(models.NewWSErrorEvent(event.RequestID, "消息已保存，实时投递失败"))
		return
	}

	if err := publishCreatedMessageUpdates(ctl.Bus, ctl.Hub, memberIDs, message); err != nil {
		wsConn.SendEvent(models.NewWSErrorEvent(event.RequestID, "消息已保存，实时投递失败"))
		return
	}
}

func (ctl WSController) presenceTracker() models.PresenceTracker {
	// PresenceTracker 可选注入。
	// 单机内存模式下用 Noop，Redis 模式下写入全局在线状态。
	if ctl.PresenceTracker != nil {
		return ctl.PresenceTracker
	}
	return models.NoopPresenceTracker{}
}

func (ctl WSController) presenceProvider() models.PresenceProvider {
	if ctl.PresenceProvider != nil {
		return ctl.PresenceProvider
	}
	return models.NewHubPresenceProvider(ctl.Hub)
}

func (ctl WSController) publishCurrentPresence(userID int64) {
	presence, err := models.GetUserPresence(ctl.presenceProvider(), userID)
	if err != nil {
		log.Printf("读取 WebSocket 在线状态失败: user_id=%d err=%v", userID, err)
		return
	}
	if err := publishPresenceChanged(ctl.Bus, ctl.Hub, models.PresenceChangedEvent{Presence: presence}); err != nil {
		log.Printf("发布 WebSocket 在线状态失败: user_id=%d err=%v", userID, err)
	}
}

// DeliverMessageToLocalHub 把实时投递事件转换成 WebSocket message.deliver。
// Redis 多实例模式下每个实例都会收到同一事件，但只会投递给本机 Hub 里真实存在的连接。
func DeliverMessageToLocalHub(hub *models.WSHub, event models.MessageDeliverEvent) {
	if hub == nil {
		return
	}

	deliverEvent := models.NewWSEvent(models.WSEventMessageDeliver, "", event.Message)
	hub.BroadcastEventToUsers(event.UserIDs, deliverEvent)
}

// DeliverPresenceChangedToLocalHub 把在线状态变化广播到当前实例的全部登录连接。
func DeliverPresenceChangedToLocalHub(hub *models.WSHub, event models.PresenceChangedEvent) {
	if hub == nil {
		return
	}
	hub.BroadcastEventToAll(models.NewWSEvent(models.WSEventPresenceChanged, "", event.Presence))
}

// DeliverConversationUnreadChangedToLocalHub 只同步给目标用户的全部本机连接。
func DeliverConversationUnreadChangedToLocalHub(hub *models.WSHub, event models.ConversationUnreadChangedEvent) {
	if hub == nil {
		return
	}
	hub.BroadcastEventToUsers([]int64{event.UserID}, models.NewWSEvent(models.WSEventConversationUnreadChanged, "", event.State))
}

// DeliverConversationChangedToLocalHub 通知受影响用户重新拉取会话列表和详情。
func DeliverConversationChangedToLocalHub(hub *models.WSHub, event models.ConversationChangedEvent) {
	if hub == nil {
		return
	}
	hub.BroadcastEventToUsers(event.UserIDs, models.NewWSEvent(models.WSEventConversationChanged, "", event.Change))
}

func publishMessageDeliver(bus models.RealtimeBus, hub *models.WSHub, event models.MessageDeliverEvent) error {
	// bus 为空时直接走本机 Hub，方便单元测试和最小化单进程部署。
	// 正常启动时路由层会注入 MemoryRealtimeBus 或 RedisRealtimeBus。
	if bus == nil {
		DeliverMessageToLocalHub(hub, event)
		return nil
	}
	return bus.PublishMessageDeliver(event)
}

func publishPresenceChanged(bus models.RealtimeBus, hub *models.WSHub, event models.PresenceChangedEvent) error {
	if bus == nil {
		DeliverPresenceChangedToLocalHub(hub, event)
		return nil
	}
	return bus.PublishPresenceChanged(event)
}

func publishConversationUnreadChanged(bus models.RealtimeBus, hub *models.WSHub, event models.ConversationUnreadChangedEvent) error {
	if bus == nil {
		DeliverConversationUnreadChangedToLocalHub(hub, event)
		return nil
	}
	return bus.PublishConversationUnreadChanged(event)
}

func publishConversationChanged(bus models.RealtimeBus, hub *models.WSHub, event models.ConversationChangedEvent) error {
	if bus == nil {
		DeliverConversationChangedToLocalHub(hub, event)
		return nil
	}
	return bus.PublishConversationChanged(event)
}

// publishCreatedMessageUpdates 先投递消息，再给每个接收者发送其权威未读状态。
// 发送者会收到 message.deliver 用于多端同步，但自己发送的消息不会增加未读数。
func publishCreatedMessageUpdates(bus models.RealtimeBus, hub *models.WSHub, memberIDs []int64, message models.MessageDTO) error {
	if err := publishMessageDeliver(bus, hub, models.MessageDeliverEvent{UserIDs: memberIDs, Message: message}); err != nil {
		return err
	}
	for _, userID := range memberIDs {
		if userID == message.SenderID {
			continue
		}
		state, err := models.GetConversationUnreadState(userID, message.ConversationID)
		if err != nil {
			return err
		}
		if err := publishConversationUnreadChanged(bus, hub, models.ConversationUnreadChangedEvent{
			UserID: userID,
			State:  state,
		}); err != nil {
			return err
		}
	}
	return nil
}

func wsMessageForError(err error) string {
	// WebSocket error 事件不能依赖 HTTP 状态码，所以这里把领域错误翻译成稳定中文文案。
	// 具体数据库错误不下发给客户端，避免泄露 SQL、索引名或内部表结构。
	switch {
	case errors.Is(err, models.ErrValidation),
		errors.Is(err, models.ErrInvalidMember),
		errors.Is(err, models.ErrInvalidMessageType),
		errors.Is(err, models.ErrInvalidMessageContent):
		return "参数校验失败"
	case errors.Is(err, models.ErrMessageForbidden),
		errors.Is(err, models.ErrConversationForbidden):
		return "无权操作该消息"
	case errors.Is(err, models.ErrMessageNotFound),
		errors.Is(err, models.ErrConversationNotFound):
		return "消息或会话不存在"
	default:
		return "服务器内部错误"
	}
}
