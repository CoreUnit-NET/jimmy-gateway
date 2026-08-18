package service

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/CoreUnit-NET/jimmy-gateway/lib/chatjimmy"
)

type chatResult struct {
	completion chatjimmy.Completion
	stream     bool
}

func (h *Handler) completeChat(r *http.Request) (*chatResult, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, badRequest("failed to read request body")
	}

	var req chatjimmy.ChatRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, badRequest("Request body must be JSON")
	}

	translated, err := chatjimmy.TranslateRequest(req, chatjimmy.TranslateOptions{})
	if err != nil {
		return nil, badRequest(err.Error())
	}

	if h.client == nil {
		return nil, upstreamError("upstream client is nil", "upstream_error")
	}

	raw, err := h.client.Chat(r.Context(), translated.Payload)
	if err != nil {
		return nil, fmtUpstreamError(err)
	}

	parsed := chatjimmy.ParseUpstream(raw)
	completion := chatjimmy.BuildCompletion(
		translated.Model,
		parsed.Text,
		parsed.Usage,
		translated.Tools,
		parsed.Stats,
	)

	return &chatResult{completion: completion, stream: req.Stream}, nil
}
