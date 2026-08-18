package chatjimmy

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
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

type BuildOptions struct {
	Model string
	Tools []Tool
}

func BuildCompletion(model, upstreamText string, usage Usage, tools []Tool) Completion {
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
		msg.Content = &upstreamText
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
		Usage: usage,
	}
}

func BuildStreamChunks(completion Completion) []StreamChunk {
	created := completion.Created
	model := completion.Model
	id := completion.ID
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
	reasonStop := "stop"
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

func EncodeSSEChunks(chunks []StreamChunk) []byte {
	var out []byte
	for _, chunk := range chunks {
		b, _ := json.Marshal(chunk)
		out = append(out, []byte("data: "+string(b)+"\n\n")...)
	}
	out = append(out, []byte("data: [DONE]\n\n")...)
	return out
}

func NewOpenAIError(message, typ, code string, status int) (OpenAIError, int) {
	_ = status
	err := OpenAIError{}
	err.Error.Message = message
	err.Error.Type = typ
	err.Error.Code = code
	return err, status
}
