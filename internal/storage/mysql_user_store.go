package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"flow-talk/internal/auth"
)

// MySQLUserStore 基于 users 表实现 auth.UserStore。
// 当前认证模块只需要创建用户和查询用户，后续会话、消息存储可以独立演进。
type MySQLUserStore struct {
	db *sql.DB
}

// NewMySQLUserStore 包装已有的数据库连接。
// 连接池参数和关闭时机由调用方负责。
func NewMySQLUserStore(db *sql.DB) *MySQLUserStore {
	return &MySQLUserStore{db: db}
}

// CreateUser 写入本地用户或外部用户镜像，并从 MySQL 重新读取。
// 重新读取可以拿到自增 ID、默认状态等数据库生成的字段。
func (s *MySQLUserStore) CreateUser(ctx context.Context, params auth.CreateUserParams) (auth.User, error) {
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO users (
			external_id,
			username,
			password,
			nickname,
			avatar_url,
			auth_source,
			status
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`,
		nullString(params.ExternalID),
		params.Username,
		nullString(params.Password),
		params.Nickname,
		nullString(params.AvatarURL),
		params.AuthSource,
		auth.UserStatusEnabled,
	)
	if err != nil {
		if isDuplicateKey(err) {
			// 当前阶段用户名是主要的用户可感知唯一冲突。
			// 后续接外部登录时，可以再把 external_id 冲突映射成更具体的错误。
			return auth.User{}, auth.ErrUsernameTaken
		}
		return auth.User{}, fmt.Errorf("create user: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return auth.User{}, fmt.Errorf("read inserted user id: %w", err)
	}
	return s.FindUserByID(ctx, id)
}

// FindUserByUsername 按用户名查询用户，用于本地登录。
func (s *MySQLUserStore) FindUserByUsername(ctx context.Context, username string) (auth.User, error) {
	return s.findOne(ctx, `
		SELECT id, external_id, username, password, nickname, avatar_url, auth_source, status
		FROM users
		WHERE username = ?
		LIMIT 1
	`, username)
}

// FindUserByID 按内部用户 ID 查询用户。
// token 校验后会重新读取用户，确保禁用状态能及时生效。
func (s *MySQLUserStore) FindUserByID(ctx context.Context, id int64) (auth.User, error) {
	return s.findOne(ctx, `
		SELECT id, external_id, username, password, nickname, avatar_url, auth_source, status
		FROM users
		WHERE id = ?
		LIMIT 1
	`, id)
}

// findOne 统一处理用户查询和字段扫描。
// sql.ErrNoRows 会被转换为认证服务能识别的 ErrUserNotFound。
func (s *MySQLUserStore) findOne(ctx context.Context, query string, args ...any) (auth.User, error) {
	var user auth.User
	var externalID sql.NullString
	var password sql.NullString
	var avatarURL sql.NullString

	err := s.db.QueryRowContext(ctx, query, args...).Scan(
		&user.ID,
		&externalID,
		&user.Username,
		&password,
		&user.Nickname,
		&avatarURL,
		&user.AuthSource,
		&user.Status,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return auth.User{}, auth.ErrUserNotFound
		}
		return auth.User{}, fmt.Errorf("find user: %w", err)
	}

	// 可空字段在领域层用空字符串表示，存储层仍保留数据库 NULL 语义。
	user.ExternalID = externalID.String
	user.Password = password.String
	user.AvatarURL = avatarURL.String
	return user, nil
}

// nullString 把可选空值写成 SQL NULL。
func nullString(value string) sql.NullString {
	if value == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: value, Valid: true}
}

// isDuplicateKey 判断 MySQL 唯一键冲突。
// 这里不把具体驱动错误类型扩散到存储边界之外。
func isDuplicateKey(err error) bool {
	message := err.Error()
	return strings.Contains(message, "Duplicate entry") || strings.Contains(message, "Error 1062")
}
