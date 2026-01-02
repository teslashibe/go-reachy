package inference

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Stream returns a streaming chat response.
func (c *Client) Stream(ctx context.Context, req *ChatRequest) (Stream, error) {
	model := req.Model
	if model == "" {
		model = c.config.Model
	}

	payload := c.buildChatPayload(req, model, true)

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, WrapError(providerClient, fmt.Errorf("marshal payload: %w", err))
	}

	url := c.baseURL + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, WrapError(providerClient, fmt.Errorf("create request: %w", err))
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	// Use stream timeout
	client := &http.Client{Timeout: c.config.StreamTimeout}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, WrapError(providerClient, fmt.Errorf("stream request: %w", err))
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		return nil, c.parseError(resp)
	}

	return &clientStream{
		reader: bufio.NewReader(resp.Body),
		body:   resp.Body,
	}, nil
}

// clientStream implements Stream for SSE responses.
type clientStream struct {
	reader *bufio.Reader
	body   io.ReadCloser
}

// Recv returns the next stream chunk.
func (s *clientStream) Recv() (*StreamChunk, error) {
	for {
		line, err := s.reader.ReadString('\n')
		if err == io.EOF {
			return &StreamChunk{Done: true}, nil
		}
		if err != nil {
			return nil, WrapError(providerClient, fmt.Errorf("read stream: %w", err))
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			return &StreamChunk{Done: true}, nil
		}

		var event streamEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			// Skip malformed events
			continue
		}

		if len(event.Choices) == 0 {
			continue
		}

		choice := event.Choices[0]
		return &StreamChunk{
			Delta:        choice.Delta.Content,
			FinishReason: choice.FinishReason,
			Done:         choice.FinishReason != "",
		}, nil
	}
}

// Close stops the stream.
func (s *clientStream) Close() error {
	return s.body.Close()
}

// streamEvent is the SSE event format.
type streamEvent struct {
	Choices []struct {
		Delta struct {
			Content   string `json:"content"`
			Role      string `json:"role"`
			ToolCalls []struct {
				ID       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

