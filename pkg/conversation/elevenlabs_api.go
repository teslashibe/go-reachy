package conversation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	elevenLabsAPIBaseURL = "https://api.elevenlabs.io/v1"
)

// AgentConfig represents the full agent configuration for the ElevenLabs API.
type AgentConfig struct {
	Name               string              `json:"name,omitempty"`
	ConversationConfig *ConversationConfig `json:"conversation_config"`
	PlatformSettings   *PlatformSettings   `json:"platform_settings,omitempty"`
}

// ConversationConfig contains the main conversation settings.
type ConversationConfig struct {
	Agent *AgentSettings `json:"agent,omitempty"`
	TTS   *TTSConfig     `json:"tts,omitempty"`
	ASR   *ASRConfig     `json:"asr,omitempty"`
	Turn  *TurnConfig    `json:"turn,omitempty"`
}

// AgentSettings configures the agent's behavior.
type AgentSettings struct {
	Prompt       *PromptConfig `json:"prompt,omitempty"`
	LLM          *LLMConfig    `json:"llm,omitempty"`
	FirstMessage string        `json:"first_message,omitempty"`
}

// PromptConfig holds the system prompt.
type PromptConfig struct {
	Prompt string `json:"prompt"`
}

// LLMConfig specifies which LLM to use.
type LLMConfig struct {
	Model string `json:"model"`
}

// TTSConfig configures text-to-speech.
type TTSConfig struct {
	VoiceID string `json:"voice_id"`
}

// ASRConfig configures automatic speech recognition.
type ASRConfig struct {
	Provider           string `json:"provider,omitempty"`
	Model              string `json:"model,omitempty"`
	Language           string `json:"language,omitempty"`
	UserInputAudioRate int    `json:"user_input_audio_rate,omitempty"`
}

// TurnConfig configures turn detection.
type TurnConfig struct {
	Mode              string `json:"mode,omitempty"`
	TurnTimeout       int    `json:"turn_timeout,omitempty"`
	SilenceDurationMs int    `json:"silence_duration_ms,omitempty"`
}

// PlatformSettings contains platform-level settings.
type PlatformSettings struct {
	Tools []AgentTool `json:"tools,omitempty"`
}

// AgentTool represents a tool definition for the agent.
type AgentTool struct {
	Type        string         `json:"type"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

// CreateAgentResponse is the response from creating an agent.
type CreateAgentResponse struct {
	AgentID string `json:"agent_id"`
}

// GetAgentResponse is the response from getting an agent.
type GetAgentResponse struct {
	AgentID            string              `json:"agent_id"`
	Name               string              `json:"name"`
	ConversationConfig *ConversationConfig `json:"conversation_config"`
	PlatformSettings   *PlatformSettings   `json:"platform_settings"`
}

// apiClient handles REST API calls to ElevenLabs.
type apiClient struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

// newAPIClient creates a new API client.
func newAPIClient(apiKey string) *apiClient {
	return &apiClient{
		apiKey:  apiKey,
		baseURL: elevenLabsAPIBaseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// CreateAgent creates a new agent via the REST API.
func (c *apiClient) CreateAgent(ctx context.Context, cfg AgentConfig) (*CreateAgentResponse, error) {
	body, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("marshal agent config: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/convai/agents/create", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("xi-api-key", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("create agent failed (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var result CreateAgentResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &result, nil
}

// GetAgent retrieves an agent by ID.
func (c *apiClient) GetAgent(ctx context.Context, agentID string) (*GetAgentResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/convai/agents/"+agentID, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("xi-api-key", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrAgentNotFound
	}

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get agent failed (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var result GetAgentResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &result, nil
}

// UpdateAgent updates an existing agent.
func (c *apiClient) UpdateAgent(ctx context.Context, agentID string, cfg AgentConfig) error {
	body, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal agent config: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, c.baseURL+"/convai/agents/"+agentID, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("xi-api-key", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("update agent failed (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}

// DeleteAgent removes an agent.
func (c *apiClient) DeleteAgent(ctx context.Context, agentID string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.baseURL+"/convai/agents/"+agentID, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("xi-api-key", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete agent failed (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}

// buildAgentConfig creates an AgentConfig from the provider Config and tools.
func buildAgentConfig(cfg *Config, tools []Tool) AgentConfig {
	agentCfg := AgentConfig{
		Name: cfg.AgentName,
		ConversationConfig: &ConversationConfig{
			Agent: &AgentSettings{
				Prompt: &PromptConfig{
					Prompt: cfg.SystemPrompt,
				},
				FirstMessage: cfg.FirstMessage,
			},
		},
	}

	// Set LLM if specified
	if cfg.LLM != "" {
		agentCfg.ConversationConfig.Agent.LLM = &LLMConfig{
			Model: cfg.LLM,
		}
	}

	// Set voice if specified
	if cfg.VoiceID != "" {
		agentCfg.ConversationConfig.TTS = &TTSConfig{
			VoiceID: cfg.VoiceID,
		}
	}

	// Set turn detection if specified
	if cfg.TurnDetection != nil {
		// ElevenLabs only supports "silence" or "turn" modes
		mode := cfg.TurnDetection.Type
		if mode == "server_vad" {
			mode = "turn" // Map OpenAI's server_vad to ElevenLabs' turn
		}
		agentCfg.ConversationConfig.Turn = &TurnConfig{
			Mode:              mode,
			SilenceDurationMs: cfg.TurnDetection.SilenceDurationMs,
		}
	}

	// Convert tools to agent tools
	if len(tools) > 0 {
		agentCfg.PlatformSettings = &PlatformSettings{
			Tools: make([]AgentTool, len(tools)),
		}
		for i, t := range tools {
			agentCfg.PlatformSettings.Tools[i] = AgentTool{
				Type:        "client",
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
			}
		}
	}

	return agentCfg
}

