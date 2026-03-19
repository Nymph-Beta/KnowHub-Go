package model

import "time"

// ChatMessage 表示一条多轮对话消息。
type ChatMessage struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"createdAt"`
}

// Conversation 表示当前会话的元信息。
// 阶段十二先只在 Redis 中维护，不落 MySQL。
type Conversation struct {
	ID        string    `json:"id"`
	UserID    uint      `json:"userId"`
	UpdatedAt time.Time `json:"updatedAt"`
}
