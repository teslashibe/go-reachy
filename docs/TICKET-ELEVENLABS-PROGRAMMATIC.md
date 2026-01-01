# TICKET: ElevenLabs Programmatic Agent Configuration

**Status:** ✅ IMPLEMENTED

---

## Summary

Refactor `pkg/conversation/elevenlabs.go` to support **fully programmatic agent configuration** via the ElevenLabs REST API, eliminating the need for dashboard-based agent setup.

## Problem

Currently, using ElevenLabs as a conversation provider requires:
1. Creating an agent manually in the ElevenLabs dashboard
2. Configuring system prompt, tools, LLM, and voice in the dashboard
3. Copying the `ELEVENLABS_AGENT_ID` to environment variables
4. Config in Go code is ignored (system prompt, tools defined in Go aren't used)

This creates friction and prevents true programmatic control:
- Duplicate configuration (dashboard vs code)
- Can't version control agent config
- Dashboard changes can break the robot

## Solution

Use the [ElevenLabs Create Agent API](https://elevenlabs.io/docs/api-reference/agents/create) to create/update agents programmatically at startup.

### API Endpoint

```
POST https://api.elevenlabs.io/v1/convai/agents
```

### Request Body

```json
{
  "conversation_config": {
    "agent": {
      "prompt": {
        "prompt": "You are Eva, a friendly robot..."
      },
      "llm": {
        "model": "gemini-2.0-flash"
      },
      "first_message": ""
    },
    "tts": {
      "voice_id": "your-voice-id"
    }
  },
  "platform_settings": {
    "tools": [
      {
        "type": "webhook",
        "name": "move_head",
        "description": "Look in a direction",
        "parameters": {...}
      }
    ]
  },
  "name": "eva-robot"
}
```

## Implementation Plan

### Phase 1: Add Agent Management API Client

Create `pkg/conversation/elevenlabs_api.go`:

```go
// AgentConfig represents the full agent configuration
type AgentConfig struct {
    Name               string            `json:"name"`
    ConversationConfig ConversationConfig `json:"conversation_config"`
    PlatformSettings   PlatformSettings   `json:"platform_settings,omitempty"`
}

type ConversationConfig struct {
    Agent AgentSettings `json:"agent"`
    TTS   TTSSettings   `json:"tts"`
    ASR   ASRSettings   `json:"asr,omitempty"`
    Turn  TurnSettings  `json:"turn,omitempty"`
}

type AgentSettings struct {
    Prompt       PromptConfig `json:"prompt"`
    LLM          LLMConfig    `json:"llm"`
    FirstMessage string       `json:"first_message,omitempty"`
}

// CreateAgent creates a new agent via the ElevenLabs API
func (e *ElevenLabs) CreateAgent(ctx context.Context, cfg AgentConfig) (string, error)

// UpdateAgent updates an existing agent
func (e *ElevenLabs) UpdateAgent(ctx context.Context, agentID string, cfg AgentConfig) error

// DeleteAgent removes an agent
func (e *ElevenLabs) DeleteAgent(ctx context.Context, agentID string) error
```

### Phase 2: Update Config Options

Update `pkg/conversation/config.go`:

```go
// New options for programmatic configuration
WithLLM(model string) Option           // "gemini-2.0-flash", "claude-3-5-sonnet", "gpt-4o"
WithVoiceID(id string) Option          // ElevenLabs voice ID
WithAgentName(name string) Option      // Agent name for dashboard reference
WithAutoCreateAgent(bool) Option       // Create agent if not exists
```

### Phase 3: Refactor ElevenLabs Provider

Update `pkg/conversation/elevenlabs.go`:

1. **On Connect**: If `AgentID` is empty and `AutoCreateAgent` is true:
   - Create agent via REST API using config from Go code
   - Store returned `agent_id`
   - Connect via WebSocket as before

2. **ConfigureSession**: Actually sends config to ElevenLabs (not a no-op)

3. **RegisterTool**: Includes tools in agent creation

### Phase 4: Remove Dashboard Dependency

Update usage pattern:

**Before (dashboard required):**
```go
provider, _ := conversation.NewElevenLabs(
    conversation.WithAPIKey(os.Getenv("ELEVENLABS_API_KEY")),
    conversation.WithAgentID(os.Getenv("ELEVENLABS_AGENT_ID")), // Must create in dashboard!
)
```

**After (fully programmatic):**
```go
provider, _ := conversation.NewElevenLabs(
    conversation.WithAPIKey(os.Getenv("ELEVENLABS_API_KEY")),
    conversation.WithVoiceID(os.Getenv("ELEVENLABS_VOICE_ID")),
    conversation.WithLLM("gemini-2.0-flash"),
    conversation.WithSystemPrompt(evaInstructions),
    conversation.WithTools(evaTools...),
    conversation.WithAutoCreateAgent(true),
)
```

### Phase 5: Update cmd/eva/main.go

Simplify Eva startup:

```go
// Before: Required ELEVENLABS_AGENT_ID from dashboard
// After: Everything from code

provider, err := conversation.NewElevenLabs(
    conversation.WithAPIKey(os.Getenv("ELEVENLABS_API_KEY")),
    conversation.WithVoiceID(os.Getenv("ELEVENLABS_VOICE_ID")),
    conversation.WithLLM("gemini-2.0-flash"),
    conversation.WithSystemPrompt(evaInstructions),
    conversation.WithAgentName("eva-robot"),
    conversation.WithAutoCreateAgent(true),
)

// Register tools
for _, tool := range realtime.EvaTools(toolsCfg) {
    provider.RegisterTool(conversation.Tool{
        Name:        tool.Name,
        Description: tool.Description,
        Parameters:  tool.Parameters,
    })
}

// Connect - agent created automatically if needed
if err := provider.Connect(ctx); err != nil {
    log.Fatal(err)
}
```

## Files to Modify

| File | Changes |
|------|---------|
| `pkg/conversation/elevenlabs.go` | Add CreateAgent, refactor Connect |
| `pkg/conversation/elevenlabs_api.go` | New file - REST API client |
| `pkg/conversation/config.go` | Add new options |
| `pkg/conversation/README.md` | Update documentation |
| `cmd/eva/main.go` | Simplify ElevenLabs setup |

## Files to Remove/Deprecate

| File/Code | Action |
|-----------|--------|
| Dashboard agent setup instructions | Remove from README |
| `WithAgentID` option | Deprecate (keep for backwards compat) |

## Testing

1. **Unit Tests**: Mock REST API responses
2. **Integration Test**: Create agent, connect, send audio, verify response
3. **Manual Test**: Run Eva with ElevenLabs + Gemini Flash

## Environment Variables

**Before:**
```bash
ELEVENLABS_API_KEY=xxx
ELEVENLABS_AGENT_ID=xxx  # Required, from dashboard
```

**After:**
```bash
ELEVENLABS_API_KEY=xxx
ELEVENLABS_VOICE_ID=xxx  # Voice ID (can get from dashboard or API)
# ELEVENLABS_AGENT_ID optional - auto-created if not set
```

## Benefits

1. ✅ **Version controlled config** - All agent config in Go code
2. ✅ **No dashboard required** - Fully programmatic
3. ✅ **Single source of truth** - Tools defined once in Go
4. ✅ **Choice of LLM** - Gemini, Claude, GPT-4o
5. ✅ **ElevenLabs voice quality** - Custom/cloned voices
6. ✅ **Backwards compatible** - `WithAgentID` still works

## Acceptance Criteria

- [ ] Can create ElevenLabs conversation without dashboard
- [ ] System prompt from Go code is used
- [ ] Tools from Go code are available
- [ ] LLM can be configured (gemini-2.0-flash, etc.)
- [ ] Voice ID configurable programmatically
- [ ] Existing `WithAgentID` flow still works
- [ ] README updated with new usage
- [ ] Integration test passes

## References

- [ElevenLabs Create Agent API](https://elevenlabs.io/docs/api-reference/agents/create)
- [ElevenLabs Agents Quickstart](https://elevenlabs.io/docs/agents-platform/quickstart)
- [ElevenLabs WebSocket API](https://elevenlabs.io/docs/agents-platform/api-reference/conversational-ai/agent-websocket)

## Priority

**High** - Removes friction for Eva development and enables LLM flexibility.

## Estimate

**2-3 days**
- Day 1: Agent API client + config options
- Day 2: Refactor ElevenLabs provider + Connect flow
- Day 3: Update Eva, testing, documentation

