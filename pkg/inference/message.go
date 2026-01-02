package inference

import "image"

// Role defines message roles in a conversation.
type Role string

const (
	// RoleSystem is for system instructions.
	RoleSystem Role = "system"

	// RoleUser is for user messages.
	RoleUser Role = "user"

	// RoleAssistant is for assistant responses.
	RoleAssistant Role = "assistant"

	// RoleTool is for tool/function results.
	RoleTool Role = "tool"
)

// Message represents a chat message in a conversation.
type Message struct {
	// Role identifies the message sender.
	Role Role

	// Content is the text content of the message.
	Content string

	// Name is optional, used for tool messages.
	Name string

	// ToolCalls are function calls requested by the assistant.
	ToolCalls []ToolCall

	// ToolCallID identifies which tool call this message responds to.
	ToolCallID string

	// Images for vision-enabled messages.
	Images []image.Image
}

// ToolCall represents a function call request from the model.
type ToolCall struct {
	// ID uniquely identifies this tool call.
	ID string

	// Name of the function to call.
	Name string

	// Arguments as a JSON string.
	Arguments string
}

// Tool defines a callable function for the model.
type Tool struct {
	// Type is always "function" for now.
	Type string

	// Function describes the callable function.
	Function ToolFunction
}

// ToolFunction describes a function the model can call.
type ToolFunction struct {
	// Name of the function.
	Name string

	// Description explains what the function does.
	Description string

	// Parameters as JSON Schema.
	Parameters map[string]interface{}
}

// NewSystemMessage creates a system message.
func NewSystemMessage(content string) Message {
	return Message{Role: RoleSystem, Content: content}
}

// NewUserMessage creates a user message.
func NewUserMessage(content string) Message {
	return Message{Role: RoleUser, Content: content}
}

// NewAssistantMessage creates an assistant message.
func NewAssistantMessage(content string) Message {
	return Message{Role: RoleAssistant, Content: content}
}

// NewToolMessage creates a tool result message.
func NewToolMessage(toolCallID, content string) Message {
	return Message{Role: RoleTool, ToolCallID: toolCallID, Content: content}
}

// NewVisionMessage creates a user message with images.
func NewVisionMessage(prompt string, images ...image.Image) Message {
	return Message{Role: RoleUser, Content: prompt, Images: images}
}

// NewTool creates a function tool definition.
func NewTool(name, description string, parameters map[string]interface{}) Tool {
	return Tool{
		Type: "function",
		Function: ToolFunction{
			Name:        name,
			Description: description,
			Parameters:  parameters,
		},
	}
}

