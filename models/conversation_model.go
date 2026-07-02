package models

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

const (
	// ConversationTypeDirect 表示一对一单聊。单聊必须写 direct_key 防重复。
	ConversationTypeDirect = "direct"
	// ConversationTypeGroup 表示群聊。群聊 direct_key 为空，owner_id 指向群主。
	ConversationTypeGroup = "group"

	// MemberRoleOwner 是群主角色。单聊不会使用 owner。
	MemberRoleOwner = "owner"
	// MemberRoleAdmin 预留给后续群管理版本。
	MemberRoleAdmin = "admin"
	// MemberRoleMember 是普通成员角色；单聊双方都使用 member。
	MemberRoleMember = "member"

	// MemberStatusActive 表示成员仍在会话中，也是读写会话的权限基础。
	MemberStatusActive = "active"
	// MemberStatusLeft / MemberStatusRemoved 预留给后续退出群聊、移除成员。
	MemberStatusLeft    = "left"
	MemberStatusRemoved = "removed"
)

var (
	// ErrConversationNotFound 表示会话本身不存在。
	ErrConversationNotFound = errors.New("会话不存在")
	// ErrConversationForbidden 表示会话存在，但当前用户不是 active 成员。
	ErrConversationForbidden = errors.New("无权访问该会话")
	// ErrInvalidConversationType 表示调用方传入了不支持的会话类型。
	ErrInvalidConversationType = errors.New("无效会话类型")
	// ErrInvalidMember 表示成员用户 ID 为空、非法或数据库中不存在。
	ErrInvalidMember = errors.New("无效会话成员")
	// ErrCannotCreateDirectWithSelf 表示用户不能和自己创建单聊。
	ErrCannotCreateDirectWithSelf = errors.New("不能和自己创建单聊")
	// ErrGroupOnly 表示当前操作只允许群聊使用。
	ErrGroupOnly = errors.New("该操作只支持群聊")
	// ErrPermissionDenied 表示当前成员角色没有管理权限。
	ErrPermissionDenied = errors.New("权限不足")
	// ErrCannotRemoveOwner 表示不能把群主从群里移除。
	ErrCannotRemoveOwner = errors.New("不能移除群主")
	// ErrOwnerCannotLeave 表示群主不能直接退出群聊。
	ErrOwnerCannotLeave = errors.New("群主不能直接退出群聊")
	// ErrInvalidMemberRole 表示角色不是 owner/admin/member 允许范围内的可变角色。
	ErrInvalidMemberRole = errors.New("无效成员角色")

	// errDirectConversationRace 是内部哨兵错误，用来处理并发创建同一个单聊时的唯一键冲突。
	errDirectConversationRace = errors.New("单聊会话已被并发创建")
)

// Conversation 映射 conversations 表，统一承载单聊和群聊。
type Conversation struct {
	ID            int64      `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	Type          string     `gorm:"column:type" json:"type"`
	DirectKey     *string    `gorm:"column:direct_key" json:"direct_key,omitempty"`
	Title         *string    `gorm:"column:title" json:"title,omitempty"`
	AvatarURL     *string    `gorm:"column:avatar_url" json:"avatar_url,omitempty"`
	OwnerID       *int64     `gorm:"column:owner_id" json:"owner_id,omitempty"`
	LastMessageID *int64     `gorm:"column:last_message_id" json:"last_message_id,omitempty"`
	LastMessageAt *time.Time `gorm:"column:last_message_at" json:"last_message_at,omitempty"`
	CreatedAt     time.Time  `gorm:"column:created_at" json:"created_at"`
	UpdatedAt     time.Time  `gorm:"column:updated_at" json:"updated_at"`
}

func (Conversation) TableName() string {
	return "conversations"
}

// ConversationMember 映射 conversation_members 表。
type ConversationMember struct {
	ID                int64      `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	ConversationID    int64      `gorm:"column:conversation_id" json:"conversation_id"`
	UserID            int64      `gorm:"column:user_id" json:"user_id"`
	Role              string     `gorm:"column:role" json:"role"`
	JoinedAt          time.Time  `gorm:"column:joined_at" json:"joined_at"`
	MutedUntil        *time.Time `gorm:"column:muted_until" json:"muted_until,omitempty"`
	LastReadMessageID *int64     `gorm:"column:last_read_message_id" json:"last_read_message_id,omitempty"`
	LastReadAt        *time.Time `gorm:"column:last_read_at" json:"last_read_at,omitempty"`
	Status            string     `gorm:"column:status" json:"status"`
}

func (ConversationMember) TableName() string {
	return "conversation_members"
}

type ConversationDTO struct {
	ID            int64                   `json:"id"`
	Type          string                  `json:"type"`
	DirectKey     string                  `json:"direct_key,omitempty"`
	Title         string                  `json:"title"`
	AvatarURL     string                  `json:"avatar_url"`
	OwnerID       int64                   `json:"owner_id"`
	MemberCount   int64                   `json:"member_count,omitempty"`
	LastMessageID int64                   `json:"last_message_id"`
	LastMessageAt string                  `json:"last_message_at"`
	Members       []ConversationMemberDTO `json:"members,omitempty"`
}

type ConversationListItemDTO struct {
	ID            int64       `json:"id"`
	Type          string      `json:"type"`
	Title         string      `json:"title"`
	AvatarURL     string      `json:"avatar_url"`
	OwnerID       int64       `json:"owner_id"`
	MemberCount   int64       `json:"member_count"`
	LastMessageID int64       `json:"last_message_id"`
	LastMessageAt string      `json:"last_message_at"`
	LastMessage   *MessageDTO `json:"last_message,omitempty"`
	UnreadCount   int64       `json:"unread_count"`
}

type ConversationDetailDTO struct {
	ID        int64                   `json:"id"`
	Type      string                  `json:"type"`
	DirectKey string                  `json:"direct_key,omitempty"`
	Title     string                  `json:"title"`
	AvatarURL string                  `json:"avatar_url"`
	OwnerID   int64                   `json:"owner_id"`
	Members   []ConversationMemberDTO `json:"members"`
}

type ConversationMemberDTO struct {
	UserID int64  `json:"user_id"`
	Role   string `json:"role"`
	Status string `json:"status"`
}

// BuildDirectKey 生成稳定单聊唯一键。
// 两个用户无论谁先发起单聊，都会得到同一个 key，例如 2 和 1 始终生成 "1:2"。
func BuildDirectKey(userID int64, targetUserID int64) (string, error) {
	if userID <= 0 || targetUserID <= 0 {
		return "", ErrInvalidMember
	}
	if userID == targetUserID {
		return "", ErrCannotCreateDirectWithSelf
	}
	if userID > targetUserID {
		userID, targetUserID = targetUserID, userID
	}
	return strconv.FormatInt(userID, 10) + ":" + strconv.FormatInt(targetUserID, 10), nil
}

// GetOrCreateDirectConversation 创建或返回已有单聊会话。
func GetOrCreateDirectConversation(userID int64, targetUserID int64) (ConversationDetailDTO, error) {
	// 单聊唯一性由 direct_key 和数据库唯一索引共同保证。
	directKey, err := BuildDirectKey(userID, targetUserID)
	if err != nil {
		return ConversationDetailDTO{}, err
	}
	// 当前 SQL 不建外键，所以写入成员关系前必须由 model 层确认用户存在。
	if err := ensureUsersExist([]int64{userID, targetUserID}); err != nil {
		return ConversationDetailDTO{}, err
	}

	existing, err := findDirectConversationByKey(directKey)
	if err == nil {
		return GetConversationDetail(userID, existing.ID)
	}
	if !errors.Is(err, ErrConversationNotFound) {
		return ConversationDetailDTO{}, err
	}

	conversation, err := createDirectConversation(userID, targetUserID, directKey)
	if errors.Is(err, errDirectConversationRace) {
		// 并发场景：另一个请求已经先插入 direct_key，当前请求转为读取已有会话。
		created, findErr := findDirectConversationByKey(directKey)
		if findErr != nil {
			return ConversationDetailDTO{}, fmt.Errorf("查询已存在单聊会话失败: %w", findErr)
		}
		return GetConversationDetail(userID, created.ID)
	}
	if err != nil {
		return ConversationDetailDTO{}, err
	}
	return GetConversationDetail(userID, conversation.ID)
}

func createDirectConversation(userID int64, targetUserID int64, directKey string) (Conversation, error) {
	conversation := Conversation{
		Type:      ConversationTypeDirect,
		DirectKey: optionalString(directKey),
	}

	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&conversation).Error; err != nil {
			if isDuplicateKey(err) {
				return errDirectConversationRace
			}
			return err
		}
		members := buildDirectMembers(conversation.ID, userID, targetUserID)
		return tx.Create(&members).Error
	})
	if err != nil {
		return Conversation{}, fmt.Errorf("创建单聊会话失败: %w", err)
	}
	return conversation, nil
}

// CreateGroupConversation 创建群聊并写入群成员。
func CreateGroupConversation(ownerID int64, title string, avatarURL string, memberIDs []int64) (ConversationDTO, error) {
	// controller 只做 binding；业务层仍要 trim，避免只传空格的群名进入数据库。
	title = strings.TrimSpace(title)
	avatarURL = strings.TrimSpace(avatarURL)
	if ownerID <= 0 || title == "" {
		return ConversationDTO{}, ErrValidation
	}

	// 当前用户必须成为群成员；调用方传入重复成员时在这里统一去重。
	allMemberIDs := buildGroupMemberIDs(ownerID, memberIDs)
	if err := ensureUsersExist(allMemberIDs); err != nil {
		return ConversationDTO{}, err
	}

	ownerIDValue := ownerID
	conversation := Conversation{
		Type:      ConversationTypeGroup,
		Title:     optionalString(title),
		AvatarURL: optionalString(avatarURL),
		OwnerID:   &ownerIDValue,
	}
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&conversation).Error; err != nil {
			return err
		}
		members := buildGroupMembers(conversation.ID, ownerID, allMemberIDs)
		return tx.Create(&members).Error
	})
	if err != nil {
		return ConversationDTO{}, fmt.Errorf("创建群聊失败: %w", err)
	}

	dto := conversation.ToDTO()
	dto.MemberCount = int64(len(allMemberIDs))
	return dto, nil
}

// ListConversations 查询当前用户 active 会话列表。
func ListConversations(userID int64) ([]ConversationListItemDTO, error) {
	if userID <= 0 {
		return nil, ErrInvalidMember
	}
	// 先查会话基础信息，再逐个补最后消息和未读数。
	// 这样 SQL 更容易读；后续会话量很大时再做批量优化。
	var rows []conversationListRow
	err := DB.Table("conversation_members AS cm").
		Select(`c.id, c.type, c.title, c.avatar_url, c.owner_id, c.last_message_id, c.last_message_at,
			(SELECT COUNT(*) FROM conversation_members cm2 WHERE cm2.conversation_id = c.id AND cm2.status = ?) AS member_count`, MemberStatusActive).
		Joins("JOIN conversations c ON c.id = cm.conversation_id").
		Where("cm.user_id = ? AND cm.status = ?", userID, MemberStatusActive).
		Order("COALESCE(c.last_message_at, c.updated_at) DESC").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("查询会话列表失败: %w", err)
	}

	result := make([]ConversationListItemDTO, 0, len(rows))
	for _, row := range rows {
		item, err := row.ToDTO(userID)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, nil
}

// GetConversationDetail 查询会话详情和成员列表。
func GetConversationDetail(userID int64, conversationID int64) (ConversationDetailDTO, error) {
	// 先查会话本体，再查成员权限。这样不存在的会话返回 404，存在但无权限返回 403。
	var conversation Conversation
	err := DB.First(&conversation, conversationID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ConversationDetailDTO{}, ErrConversationNotFound
	}
	if err != nil {
		return ConversationDetailDTO{}, fmt.Errorf("查询会话失败: %w", err)
	}
	// 会话详情只允许 active 成员查看，退出或被移除的成员不再有权限。
	if _, err := EnsureActiveMember(userID, conversationID); err != nil {
		return ConversationDetailDTO{}, err
	}

	var members []ConversationMember
	if err := DB.Where("conversation_id = ?", conversationID).Order("id asc").Find(&members).Error; err != nil {
		return ConversationDetailDTO{}, fmt.Errorf("查询会话成员失败: %w", err)
	}

	return conversation.ToDetailDTO(members), nil
}

// EnsureActiveMember 校验用户是否是会话 active 成员。
// 后续发送消息、读消息、群管理等能力都可以复用这个权限入口。
func EnsureActiveMember(userID int64, conversationID int64) (ConversationMember, error) {
	if userID <= 0 || conversationID <= 0 {
		return ConversationMember{}, ErrInvalidMember
	}
	var member ConversationMember
	err := DB.Where("conversation_id = ? AND user_id = ? AND status = ?", conversationID, userID, MemberStatusActive).First(&member).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ConversationMember{}, ErrConversationForbidden
	}
	if err != nil {
		return ConversationMember{}, fmt.Errorf("查询会话成员失败: %w", err)
	}
	return member, nil
}

// ListActiveConversationMemberIDs 返回某个会话内所有 active 成员 ID。
// WebSocket 投递、回执写入和后续通知逻辑都使用这个方法，避免各处重复拼接成员查询 SQL。
func ListActiveConversationMemberIDs(conversationID int64) ([]int64, error) {
	if conversationID <= 0 {
		return nil, ErrConversationNotFound
	}
	var userIDs []int64
	err := DB.Model(&ConversationMember{}).
		Where("conversation_id = ? AND status = ?", conversationID, MemberStatusActive).
		Order("id asc").
		Pluck("user_id", &userIDs).Error
	if err != nil {
		return nil, fmt.Errorf("查询会话成员失败: %w", err)
	}
	return userIDs, nil
}

// EnsureGroupConversation 查询并确认会话是群聊。
// 群管理接口必须先走这个方法，避免单聊误用群成员管理能力。
func EnsureGroupConversation(conversationID int64) (Conversation, error) {
	if conversationID <= 0 {
		return Conversation{}, ErrConversationNotFound
	}
	var conversation Conversation
	err := DB.First(&conversation, conversationID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Conversation{}, ErrConversationNotFound
	}
	if err != nil {
		return Conversation{}, fmt.Errorf("查询会话失败: %w", err)
	}
	if conversation.Type != ConversationTypeGroup {
		return Conversation{}, ErrGroupOnly
	}
	return conversation, nil
}

// AddGroupMembers 添加群成员，包含“已退出/被移除成员重新激活”的逻辑。
func AddGroupMembers(operatorID int64, conversationID int64, userIDs []int64) ([]ConversationMemberDTO, error) {
	if len(userIDs) == 0 {
		return nil, ErrInvalidMember
	}
	userIDs = uniquePositiveIDs(userIDs)
	if len(userIDs) == 0 {
		return nil, ErrInvalidMember
	}
	if err := ensureUsersExist(userIDs); err != nil {
		return nil, err
	}

	var result []ConversationMemberDTO
	err := DB.Transaction(func(tx *gorm.DB) error {
		if _, err := ensureGroupConversationWithDB(tx, conversationID); err != nil {
			return err
		}
		operator, err := ensureActiveMemberWithConversationDB(tx, operatorID, conversationID)
		if err != nil {
			return err
		}
		if operator.Role != MemberRoleOwner && operator.Role != MemberRoleAdmin {
			return ErrPermissionDenied
		}

		members := make([]ConversationMember, 0, len(userIDs))
		for _, userID := range userIDs {
			member, err := findMemberWithDB(tx, conversationID, userID)
			if errors.Is(err, ErrInvalidMember) {
				member = ConversationMember{
					ConversationID: conversationID,
					UserID:         userID,
					Role:           MemberRoleMember,
					Status:         MemberStatusActive,
				}
				if err := tx.Create(&member).Error; err != nil {
					return err
				}
				members = append(members, member)
				continue
			}
			if err != nil {
				return err
			}
			if member.Status != MemberStatusActive {
				if err := tx.Model(&member).Updates(map[string]any{
					"role":      MemberRoleMember,
					"status":    MemberStatusActive,
					"joined_at": time.Now(),
				}).Error; err != nil {
					return err
				}
				member.Role = MemberRoleMember
				member.Status = MemberStatusActive
			}
			members = append(members, member)
		}

		result = make([]ConversationMemberDTO, 0, len(members))
		for _, member := range members {
			result = append(result, member.ToDTO())
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("添加群成员失败: %w", err)
	}
	return result, nil
}

func RemoveGroupMember(operatorID int64, conversationID int64, targetUserID int64) error {
	if targetUserID <= 0 {
		return ErrInvalidMember
	}
	err := DB.Transaction(func(tx *gorm.DB) error {
		if _, err := ensureGroupConversationWithDB(tx, conversationID); err != nil {
			return err
		}
		operator, err := ensureActiveMemberWithConversationDB(tx, operatorID, conversationID)
		if err != nil {
			return err
		}
		target, err := ensureActiveMemberWithConversationDB(tx, targetUserID, conversationID)
		if err != nil {
			return err
		}
		if target.Role == MemberRoleOwner {
			return ErrCannotRemoveOwner
		}
		if !CanManageGroupMember(operator, target) {
			return ErrPermissionDenied
		}
		return tx.Model(&target).Update("status", MemberStatusRemoved).Error
	})
	if err != nil {
		return fmt.Errorf("移除群成员失败: %w", err)
	}
	return nil
}

func LeaveGroup(userID int64, conversationID int64) error {
	err := DB.Transaction(func(tx *gorm.DB) error {
		if _, err := ensureGroupConversationWithDB(tx, conversationID); err != nil {
			return err
		}
		member, err := ensureActiveMemberWithConversationDB(tx, userID, conversationID)
		if err != nil {
			return err
		}
		if member.Role == MemberRoleOwner {
			return ErrOwnerCannotLeave
		}
		return tx.Model(&member).Update("status", MemberStatusLeft).Error
	})
	if err != nil {
		return fmt.Errorf("退出群聊失败: %w", err)
	}
	return nil
}

func UpdateMemberRole(operatorID int64, conversationID int64, targetUserID int64, role string) (ConversationMemberDTO, error) {
	role = strings.TrimSpace(role)
	if role != MemberRoleAdmin && role != MemberRoleMember {
		return ConversationMemberDTO{}, ErrInvalidMemberRole
	}

	var updated ConversationMember
	err := DB.Transaction(func(tx *gorm.DB) error {
		if _, err := ensureGroupConversationWithDB(tx, conversationID); err != nil {
			return err
		}
		operator, err := ensureActiveMemberWithConversationDB(tx, operatorID, conversationID)
		if err != nil {
			return err
		}
		if operator.Role != MemberRoleOwner {
			return ErrPermissionDenied
		}
		target, err := ensureActiveMemberWithConversationDB(tx, targetUserID, conversationID)
		if err != nil {
			return err
		}
		if target.Role == MemberRoleOwner {
			return ErrInvalidMemberRole
		}
		if err := tx.Model(&target).Update("role", role).Error; err != nil {
			return err
		}
		target.Role = role
		updated = target
		return nil
	})
	if err != nil {
		return ConversationMemberDTO{}, fmt.Errorf("更新成员角色失败: %w", err)
	}
	return updated.ToDTO(), nil
}

func UpdateGroupProfile(operatorID int64, conversationID int64, title string, avatarURL string) (ConversationDTO, error) {
	title = strings.TrimSpace(title)
	avatarURL = strings.TrimSpace(avatarURL)
	if title == "" {
		return ConversationDTO{}, ErrValidation
	}

	var conversation Conversation
	err := DB.Transaction(func(tx *gorm.DB) error {
		found, err := ensureGroupConversationWithDB(tx, conversationID)
		if err != nil {
			return err
		}
		operator, err := ensureActiveMemberWithConversationDB(tx, operatorID, conversationID)
		if err != nil {
			return err
		}
		if operator.Role != MemberRoleOwner && operator.Role != MemberRoleAdmin {
			return ErrPermissionDenied
		}
		if err := tx.Model(&found).Updates(map[string]any{
			"title":      optionalString(title),
			"avatar_url": optionalString(avatarURL),
		}).Error; err != nil {
			return err
		}
		found.Title = optionalString(title)
		found.AvatarURL = optionalString(avatarURL)
		conversation = found
		return nil
	})
	if err != nil {
		return ConversationDTO{}, fmt.Errorf("修改群资料失败: %w", err)
	}
	return conversation.ToDTO(), nil
}

// CanManageGroupMember 表达 v6 权限矩阵中“操作者能否管理目标成员”的规则。
func CanManageGroupMember(operator ConversationMember, target ConversationMember) bool {
	if operator.Status != MemberStatusActive || target.Status != MemberStatusActive {
		return false
	}
	if operator.Role == MemberRoleOwner {
		return target.Role != MemberRoleOwner
	}
	if operator.Role == MemberRoleAdmin {
		return target.Role == MemberRoleMember
	}
	return false
}

// ToDTO 把数据库模型转换为接口模型，并把 NULL 字段转成前端更容易消费的零值。
func (c Conversation) ToDTO() ConversationDTO {
	return ConversationDTO{
		ID:            c.ID,
		Type:          c.Type,
		DirectKey:     stringValue(c.DirectKey),
		Title:         stringValue(c.Title),
		AvatarURL:     stringValue(c.AvatarURL),
		OwnerID:       int64Value(c.OwnerID),
		LastMessageID: int64Value(c.LastMessageID),
		LastMessageAt: timeString(c.LastMessageAt),
	}
}

// ToDetailDTO 在会话基础信息上附加成员列表。
func (c Conversation) ToDetailDTO(members []ConversationMember) ConversationDetailDTO {
	result := ConversationDetailDTO{
		ID:        c.ID,
		Type:      c.Type,
		DirectKey: stringValue(c.DirectKey),
		Title:     stringValue(c.Title),
		AvatarURL: stringValue(c.AvatarURL),
		OwnerID:   int64Value(c.OwnerID),
		Members:   make([]ConversationMemberDTO, 0, len(members)),
	}
	for _, member := range members {
		result.Members = append(result.Members, member.ToDTO())
	}
	return result
}

// ToDTO 输出成员的外部可见字段，不暴露内部记录 ID。
func (m ConversationMember) ToDTO() ConversationMemberDTO {
	return ConversationMemberDTO{
		UserID: m.UserID,
		Role:   m.Role,
		Status: m.Status,
	}
}

// conversationListRow 是会话列表 SQL 的扫描目标。
// 独立于 Conversation 可以避免把 member_count 这种查询派生字段塞进表模型。
type conversationListRow struct {
	ID            int64
	Type          string
	Title         *string
	AvatarURL     *string
	OwnerID       *int64
	LastMessageID *int64
	LastMessageAt *time.Time
	MemberCount   int64
}

// ToDTO 把列表查询行转换成稳定的响应结构。
func (r conversationListRow) ToDTO(userID int64) (ConversationListItemDTO, error) {
	item := ConversationListItemDTO{
		ID:            r.ID,
		Type:          r.Type,
		Title:         stringValue(r.Title),
		AvatarURL:     stringValue(r.AvatarURL),
		OwnerID:       int64Value(r.OwnerID),
		MemberCount:   r.MemberCount,
		LastMessageID: int64Value(r.LastMessageID),
		LastMessageAt: timeString(r.LastMessageAt),
	}

	lastMessage, err := findOptionalLastMessage(r.LastMessageID)
	if err != nil {
		return ConversationListItemDTO{}, err
	}
	item.LastMessage = lastMessage

	unreadCount, err := countUnreadMessages(userID, r.ID)
	if err != nil {
		return ConversationListItemDTO{}, err
	}
	item.UnreadCount = unreadCount
	return item, nil
}

func findDirectConversationByKey(directKey string) (Conversation, error) {
	var conversation Conversation
	err := DB.Where("direct_key = ?", directKey).First(&conversation).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Conversation{}, ErrConversationNotFound
	}
	if err != nil {
		return Conversation{}, fmt.Errorf("查询单聊会话失败: %w", err)
	}
	return conversation, nil
}

func findOptionalLastMessage(lastMessageID *int64) (*MessageDTO, error) {
	if lastMessageID == nil || *lastMessageID <= 0 {
		return nil, nil
	}
	message, err := FindMessageByID(*lastMessageID)
	if errors.Is(err, ErrMessageNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	dto := message.ToDTO()
	return &dto, nil
}

func countUnreadMessages(userID int64, conversationID int64) (int64, error) {
	var count int64
	err := DB.Table("messages AS m").
		Joins("JOIN conversation_members cm ON cm.conversation_id = m.conversation_id").
		Where("cm.conversation_id = ? AND cm.user_id = ? AND cm.status = ?", conversationID, userID, MemberStatusActive).
		Where("m.sender_id <> ?", userID).
		Where("m.id > COALESCE(cm.last_read_message_id, 0)").
		Where("m.status = ?", MessageStatusNormal).
		Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("计算未读数失败: %w", err)
	}
	return count, nil
}

func buildDirectMembers(conversationID int64, userID int64, targetUserID int64) []ConversationMember {
	return []ConversationMember{
		{ConversationID: conversationID, UserID: userID, Role: MemberRoleMember, Status: MemberStatusActive},
		{ConversationID: conversationID, UserID: targetUserID, Role: MemberRoleMember, Status: MemberStatusActive},
	}
}

func buildGroupMembers(conversationID int64, ownerID int64, memberIDs []int64) []ConversationMember {
	members := make([]ConversationMember, 0, len(memberIDs))
	for _, memberID := range memberIDs {
		role := MemberRoleMember
		if memberID == ownerID {
			role = MemberRoleOwner
		}
		members = append(members, ConversationMember{
			ConversationID: conversationID,
			UserID:         memberID,
			Role:           role,
			Status:         MemberStatusActive,
		})
	}
	return members
}

// buildGroupMemberIDs 生成群成员 ID 集合。
// 这里会自动补齐 owner、去掉非法 ID、去重并排序，保证后续写库顺序稳定。
func buildGroupMemberIDs(ownerID int64, memberIDs []int64) []int64 {
	seen := map[int64]bool{}
	result := make([]int64, 0, len(memberIDs)+1)
	for _, id := range append(memberIDs, ownerID) {
		if id <= 0 || seen[id] {
			continue
		}
		seen[id] = true
		result = append(result, id)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i] < result[j]
	})
	return result
}

func uniquePositiveIDs(ids []int64) []int64 {
	seen := map[int64]bool{}
	result := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 || seen[id] {
			continue
		}
		seen[id] = true
		result = append(result, id)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i] < result[j]
	})
	return result
}

func ensureGroupConversationWithDB(db *gorm.DB, conversationID int64) (Conversation, error) {
	if conversationID <= 0 {
		return Conversation{}, ErrConversationNotFound
	}
	var conversation Conversation
	err := db.First(&conversation, conversationID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Conversation{}, ErrConversationNotFound
	}
	if err != nil {
		return Conversation{}, fmt.Errorf("查询会话失败: %w", err)
	}
	if conversation.Type != ConversationTypeGroup {
		return Conversation{}, ErrGroupOnly
	}
	return conversation, nil
}

func ensureActiveMemberWithConversationDB(db *gorm.DB, userID int64, conversationID int64) (ConversationMember, error) {
	if userID <= 0 || conversationID <= 0 {
		return ConversationMember{}, ErrInvalidMember
	}
	var member ConversationMember
	err := db.Where("conversation_id = ? AND user_id = ? AND status = ?", conversationID, userID, MemberStatusActive).First(&member).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ConversationMember{}, ErrConversationForbidden
	}
	if err != nil {
		return ConversationMember{}, fmt.Errorf("查询会话成员失败: %w", err)
	}
	return member, nil
}

func findMemberWithDB(db *gorm.DB, conversationID int64, userID int64) (ConversationMember, error) {
	if conversationID <= 0 || userID <= 0 {
		return ConversationMember{}, ErrInvalidMember
	}
	var member ConversationMember
	err := db.Where("conversation_id = ? AND user_id = ?", conversationID, userID).First(&member).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ConversationMember{}, ErrInvalidMember
	}
	if err != nil {
		return ConversationMember{}, fmt.Errorf("查询会话成员失败: %w", err)
	}
	return member, nil
}

// ensureUsersExist 检查用户 ID 是否都能在 users 表中找到。
// 因为当前建表 SQL 没有外键，所以关系完整性必须在 model 层显式保证。
func ensureUsersExist(userIDs []int64) error {
	if len(userIDs) == 0 {
		return ErrInvalidMember
	}
	for _, id := range userIDs {
		if id <= 0 {
			return ErrInvalidMember
		}
	}
	var count int64
	if err := DB.Model(&User{}).Where("id IN ?", userIDs).Count(&count).Error; err != nil {
		return fmt.Errorf("查询用户失败: %w", err)
	}
	if count != int64(len(userIDs)) {
		return ErrInvalidMember
	}
	return nil
}

// int64Value 把数据库可空 BIGINT 转成接口层零值。
func int64Value(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}

// timeString 把可空时间转成 RFC3339 字符串；NULL 返回空字符串。
func timeString(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.Format(time.RFC3339)
}
