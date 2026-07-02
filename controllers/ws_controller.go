package controllers

import (
	"encoding/json"
	"errors"
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
)

// WSController 处理 WebSocket 建连和事件分发。
type WSController struct {
	JWT models.JWTConfig
	Hub *models.WSHub
}

var wsUpgrader = websocket.Upgrader{
	// 当前项目暂未做浏览器来源白名单；生产环境应按前端域名收紧。
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Connect 处理 GET /ws?token={jwt}&device_id={device_id}。
// WebSocket 无法稳定携带 Authorization 头，所以这里直接解析 query token。
func (ctl WSController) Connect(c *gin.Context) {
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
	if wsConn.DeviceID != "" {
		// 设备表属于 v5 能力；这里忽略“不存在设备”的错误，让纯 WebSocket 调试不被设备上报流程阻塞。
		_ = models.TouchUserDevice(user.ID, wsConn.DeviceID)
	}

	go ctl.writeLoop(socket, wsConn)
	ctl.readLoop(user, socket, wsConn)
}

func (ctl WSController) userFromQueryToken(c *gin.Context) (models.User, bool) {
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

	user, err := models.FindUserByID(claims.UserID)
	if err != nil || user.Status != models.UserStatusEnabled {
		c.AbortWithStatus(http.StatusUnauthorized)
		return models.User{}, false
	}
	return user, true
}

func (ctl WSController) readLoop(user models.User, socket *websocket.Conn, wsConn *models.WSConnection) {
	defer func() {
		ctl.Hub.Remove(wsConn.UserID, wsConn.ID)
		_ = socket.Close()
	}()

	socket.SetReadLimit(wsReadLimit)
	for {
		_, payload, err := socket.ReadMessage()
		if err != nil {
			return
		}

		ctl.Hub.Touch(wsConn.UserID, wsConn.ID)
		if wsConn.DeviceID != "" {
			_ = models.TouchUserDevice(wsConn.UserID, wsConn.DeviceID)
		}

		var event models.WSEvent
		if err := json.Unmarshal(payload, &event); err != nil {
			wsConn.SendEvent(models.NewWSErrorEvent("", "无效事件格式"))
			continue
		}
		ctl.handleEvent(user, event, wsConn)
	}
}

func (ctl WSController) writeLoop(socket *websocket.Conn, wsConn *models.WSConnection) {
	for payload := range wsConn.Send {
		if err := socket.SetWriteDeadline(time.Now().Add(wsWriteWait)); err != nil {
			return
		}
		if err := socket.WriteMessage(websocket.TextMessage, payload); err != nil {
			return
		}
	}
}

func (ctl WSController) handleEvent(user models.User, event models.WSEvent, wsConn *models.WSConnection) {
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

	message, err := models.SendMessage(user.ID, req.ConversationID, req.ClientMsgID, req.MessageType, req.Content)
	if err != nil {
		wsConn.SendEvent(models.NewWSErrorEvent(event.RequestID, wsMessageForError(err)))
		return
	}

	// ack 只回给当前连接，表示客户端这次发送请求已经完成入库。
	wsConn.SendEvent(models.NewWSEvent(models.WSEventMessageAck, event.RequestID, message))

	memberIDs, err := models.ListActiveConversationMemberIDs(message.ConversationID)
	if err != nil {
		wsConn.SendEvent(models.NewWSErrorEvent(event.RequestID, "消息已保存，实时投递失败"))
		return
	}

	deliverEvent := models.NewWSEvent(models.WSEventMessageDeliver, "", message)
	ctl.Hub.BroadcastEventToUsers(memberIDs, deliverEvent)

	// v7 回执能力存在后，只有本机在线用户才标记 delivered。
	for _, userID := range ctl.Hub.OnlineUserIDs(memberIDs) {
		if userID != message.SenderID {
			_ = models.MarkMessageDelivered(message.ID, userID)
		}
	}
}

func wsMessageForError(err error) string {
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
