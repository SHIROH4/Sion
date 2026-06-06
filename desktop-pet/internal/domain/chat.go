package domain

// ChatProcessor processes chat messages before and after LLM calls.
type ChatProcessor interface {
	OnBeforeChat(ctx *ChatContext) error
	OnAfterChat(ctx *ChatContext) error
}

// MessageFilter filters or transforms messages.
type MessageFilter interface {
	FilterMessage(msg *Message) error
}

// ChatPipeline is the chat processing pipeline exposed to plugins.
type ChatPipeline interface {
	ProcessChat(input string, llmCall func([]Message, func(string) error) error) (*ChatContext, error)
}
