package voice

// Tool represents a function that the AI can invoke during conversation.
// Tools enable the AI to perform actions like controlling robot movement,
// accessing cameras, or searching the web.
type Tool struct {
	// Name is the unique identifier for the tool (e.g., "describe_scene").
	Name string `json:"name"`

	// Description explains what the tool does, helping the AI decide when to use it.
	Description string `json:"description"`

	// Parameters defines the JSON schema for the tool's arguments.
	// Example:
	//   map[string]any{
	//       "type": "object",
	//       "properties": map[string]any{
	//           "direction": map[string]any{
	//               "type": "string",
	//               "enum": []string{"left", "right", "up", "down"},
	//           },
	//       },
	//       "required": []string{"direction"},
	//   }
	Parameters map[string]any `json:"parameters"`

	// Handler is called when the AI invokes this tool.
	// It receives the parsed arguments and returns a result string or error.
	// The result is sent back to the AI to continue the conversation.
	Handler func(args map[string]any) (string, error) `json:"-"`
}

// ToolCall represents an invocation of a tool by the AI.
type ToolCall struct {
	// ID is the unique identifier for this tool call.
	// Used to match results back to the correct call.
	ID string

	// Name is the tool being invoked.
	Name string

	// Arguments contains the parsed arguments from the AI.
	Arguments map[string]any
}

// ToolResult represents the result of a tool invocation.
type ToolResult struct {
	// CallID matches the ToolCall.ID this result corresponds to.
	CallID string

	// Result is the string result to send back to the AI.
	Result string

	// Error is set if the tool execution failed.
	Error error
}

