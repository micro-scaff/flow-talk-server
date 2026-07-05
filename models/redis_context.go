package models

import (
	"context"
)

// redisContext 返回当前 Redis 操作使用的 context。
// 先集中成 helper，后续如果要加超时、trace id 或请求级 context，可以只改这个边界。
func redisContext() context.Context {
	return context.Background()
}
