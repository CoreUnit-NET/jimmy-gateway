package chatjimmy

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

func NewCompletionID() string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano())
	}
	return "chatcmpl-" + hex.EncodeToString(buf)
}

func NewToolCallID() string {
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("call_%d", time.Now().UnixNano())
	}
	return "call_" + hex.EncodeToString(buf)
}

func BuildCompletion(model, upstreamText string, usage Usage, tools []Tool, stats map[string]any) Completion {
	text, toolCalls := ParseToolCalls(upstreamText, tools, NewToolCallID)
	created := time.Now().Unix()

	msg := AssistantMessage{Role: "assistant"}
	finishReason := "stop"
	if len(toolCalls) > 0 {
		finishReason = "tool_calls"
		if text != "" {
			msg.Content = &text
		}
		msg.ToolCalls = toolCalls
	} else {
		msg.Content = &text
		finishReason = finishReasonFromStats(stats)
	}

	var statsOut any
	if len(stats) > 0 {
		statsOut = stats
	}

	return Completion{
		ID:      NewCompletionID(),
		Object:  "chat.completion",
		Created: created,
		Model:   model,
		Choices: []CompletionChoice{{
			Index:        0,
			Message:      msg,
			FinishReason: finishReason,
		}},
		Usage:          usage,
		ChatJimmyStats: statsOut,
	}
}

func finishReasonFromStats(stats map[string]any) string {
	if len(stats) == 0 {
		return "stop"
	}
	for _, key := range []string{"done_reason", "reason"} {
		value, ok := stats[key]
		if !ok || value == nil {
			continue
		}
		s := strings.ToLower(fmt.Sprint(value))
		if strings.Contains(s, "length") || strings.Contains(s, "max_token") {
			return "length"
		}
	}
	return "stop"
}

func BuildStreamChunks(completion Completion) []StreamChunk {
	created := completion.Created
	model := completion.Model
	id := completion.ID
	if len(completion.Choices) == 0 {
		reason := "stop"
		return []StreamChunk{{
			ID: id, Object: "chat.completion.chunk", Created: created, Model: model,
			Choices: []StreamChunkChoice{{
				Index:        0,
				Delta:        StreamChunkDelta{Role: "assistant"},
				FinishReason: &reason,
			}},
		}}
	}
	choice := completion.Choices[0]
	msg := choice.Message

	if len(msg.ToolCalls) > 0 {
		chunks := []StreamChunk{{
			ID: id, Object: "chat.completion.chunk", Created: created, Model: model,
			Choices: []StreamChunkChoice{{
				Index: 0,
				Delta: StreamChunkDelta{Role: "assistant", Content: ""},
			}},
		}}
		if msg.Content != nil && *msg.Content != "" {
			chunks = append(chunks, StreamChunk{
				ID: id, Object: "chat.completion.chunk", Created: created, Model: model,
				Choices: []StreamChunkChoice{{
					Index: 0,
					Delta: StreamChunkDelta{Content: *msg.Content},
				}},
			})
		}
		for i, tc := range msg.ToolCalls {
			idx := i
			chunks = append(chunks, StreamChunk{
				ID: id, Object: "chat.completion.chunk", Created: created, Model: model,
				Choices: []StreamChunkChoice{{
					Index: 0,
					Delta: StreamChunkDelta{
						ToolCalls: []ToolCall{{
							Index: &idx,
							ID:    tc.ID,
							Type:  tc.Type,
							Function: ToolCallFunction{
								Name:      tc.Function.Name,
								Arguments: tc.Function.Arguments,
							},
						}},
					},
				}},
			})
		}
		reason := "tool_calls"
		chunks = append(chunks, StreamChunk{
			ID: id, Object: "chat.completion.chunk", Created: created, Model: model,
			Choices: []StreamChunkChoice{{
				Index:        0,
				Delta:        StreamChunkDelta{},
				FinishReason: &reason,
			}},
		})
		return chunks
	}

	content := ""
	if msg.Content != nil {
		content = *msg.Content
	}
	reasonStop := choice.FinishReason
	if reasonStop == "" {
		reasonStop = "stop"
	}
	return []StreamChunk{
		{
			ID: id, Object: "chat.completion.chunk", Created: created, Model: model,
			Choices: []StreamChunkChoice{{
				Index: 0,
				Delta: StreamChunkDelta{Role: "assistant"},
			}},
		},
		{
			ID: id, Object: "chat.completion.chunk", Created: created, Model: model,
			Choices: []StreamChunkChoice{{
				Index: 0,
				Delta: StreamChunkDelta{Content: content},
			}},
		},
		{
			ID: id, Object: "chat.completion.chunk", Created: created, Model: model,
			Choices: []StreamChunkChoice{{
				Index:        0,
				Delta:        StreamChunkDelta{},
				FinishReason: &reasonStop,
			}},
		},
	}
}

// AppendUsageChunk adds a final OpenAI stream chunk with usage and empty choices.
// Call only when the client requested stream_options.include_usage.
func AppendUsageChunk(chunks []StreamChunk, completion Completion) []StreamChunk {
	usage := completion.Usage
	return append(chunks, StreamChunk{
		ID:      completion.ID,
		Object:  "chat.completion.chunk",
		Created: completion.Created,
		Model:   completion.Model,
		Choices: []StreamChunkChoice{},
		Usage:   &usage,
	})
}

func EncodeSSEChunks(chunks []StreamChunk) []byte {
	var out []byte
	for _, chunk := range chunks {
		b, _ := MarshalJSON(chunk)
		out = append(out, []byte("data: "+string(b)+"\n\n")...)
	}
	out = append(out, []byte("data: [DONE]\n\n")...)
	return out
}

func NewOpenAIError(message, typ, code string) OpenAIError {
	err := OpenAIError{}
	err.Error.Message = message
	err.Error.Type = typ
	err.Error.Code = code
	return err
}
