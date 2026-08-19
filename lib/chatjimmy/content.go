package chatjimmy

import (
	"encoding/json"
	"strings"
)

func ContentToText(raw json.RawMessage) string {
	text, _ := ParseContent(raw)
	return text
}

func ParseContent(raw json.RawMessage) (string, *Attachment) {
	if isEmptyJSON(raw) {
		return "", nil
	}

	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s, nil
	}

	var parts []json.RawMessage
	if err := json.Unmarshal(raw, &parts); err == nil {
		texts := make([]string, 0, len(parts))
		var att *Attachment
		for _, part := range parts {
			text, partAtt := parseContentPart(part)
			if text != "" {
				texts = append(texts, text)
			}
			if att == nil && partAtt != nil {
				att = partAtt
			}
		}
		return strings.Join(texts, "\n"), att
	}

	return parseContentPart(raw)
}

func parseContentPart(part json.RawMessage) (string, *Attachment) {
	var s string
	if err := json.Unmarshal(part, &s); err == nil {
		return s, nil
	}

	var obj map[string]json.RawMessage
	if err := json.Unmarshal(part, &obj); err != nil {
		return "", nil
	}
	if att := attachmentFromObject(obj); att != nil {
		return "", att
	}
	if t, ok := obj["text"]; ok {
		return ParseContent(t)
	}
	if c, ok := obj["content"]; ok {
		return ParseContent(c)
	}
	return "", nil
}

func attachmentFromObject(obj map[string]json.RawMessage) *Attachment {
	if raw, ok := obj["inlineData"]; ok {
		var inline struct {
			MimeType string `json:"mimeType"`
			Data     string `json:"data"`
		}
		if json.Unmarshal(raw, &inline) == nil && strings.TrimSpace(inline.Data) != "" {
			mime := strings.TrimSpace(inline.MimeType)
			if mime == "" {
				mime = "image/png"
			}
			return &Attachment{Type: "image", Data: inline.Data, MimeType: mime}
		}
	}

	if raw, ok := obj["image_url"]; ok {
		return attachmentFromImageURL(raw)
	}

	var typ string
	_ = json.Unmarshal(obj["type"], &typ)
	if typ == "image_url" || typ == "input_image" || typ == "image" {
		if raw, ok := obj["url"]; ok {
			return attachmentFromImageURL(raw)
		}
	}
	return nil
}

func attachmentFromImageURL(raw json.RawMessage) *Attachment {
	var url string
	if json.Unmarshal(raw, &url) != nil {
		var obj struct {
			URL string `json:"url"`
		}
		if json.Unmarshal(raw, &obj) != nil {
			return nil
		}
		url = obj.URL
	}
	return parseDataURL(url)
}

func parseDataURL(url string) *Attachment {
	url = strings.TrimSpace(url)
	if !strings.HasPrefix(url, "data:") {
		return nil
	}
	rest := strings.TrimPrefix(url, "data:")
	mime, data, ok := strings.Cut(rest, ",")
	if !ok || strings.TrimSpace(data) == "" {
		return nil
	}
	mime = strings.TrimSpace(mime)
	mime = strings.TrimSuffix(mime, ";base64")
	mime = strings.TrimSpace(mime)
	if mime == "" {
		mime = "image/png"
	}
	return &Attachment{Type: "image", Data: data, MimeType: mime}
}
