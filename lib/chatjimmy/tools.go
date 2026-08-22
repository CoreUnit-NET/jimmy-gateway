package chatjimmy

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var toolCallPattern = regexp.MustCompile(`(?s)<tool_call>\s*(.*?)\s*</tool_call>`)
var unclosedToolCallPattern = regexp.MustCompile(`(?s)<tool_call\b[^>]*>.*$`)

func FilterTools(tools []Tool) []Tool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]Tool, 0, len(tools))
	for _, tool := range tools {
		name := strings.ToLower(tool.Function.Name)
		if _, skip := FilteredTools[name]; skip {
			continue
		}
		out = append(out, tool)
	}
	return out
}

func firstSentence(text string) string {
	if text == "" {
		return ""
	}
	for _, end := range []string{". ", ".\n", "\n"} {
		if idx := strings.Index(text, end); idx >= 0 {
			return strings.TrimSpace(text[:idx+1])
		}
	}
	if len(text) > 120 {
		return strings.TrimSpace(text[:120])
	}
	return strings.TrimSpace(text)
}

func FormatToolsForPrompt(tools []Tool, toolChoice json.RawMessage) string {
	if len(tools) == 0 {
		return ""
	}
	if parseToolChoice(toolChoice) == "none" {
		return ""
	}

	// Use real tool+parameter names in the example so the model doesn't copy placeholders.
	exampleToolName := tools[0].Function.Name
	exampleParamKey := "required_param"
	schema := parseParameters(tools[0].Function.Parameters)
	if len(schema.Required) > 0 {
		exampleParamKey = schema.Required[0]
	} else if len(schema.Properties) > 0 {
		for k := range schema.Properties {
			exampleParamKey = k
			break
		}
	}

	lines := []string{
		"",
		"# Tools",
		"When you need a tool, respond with one or more <tool_call> blocks and nothing else.",
		"Format:",
		"<tool_call>",
		fmt.Sprintf(`{"name": %q, "arguments": {%q: "value"}}`, exampleToolName, exampleParamKey),
		"</tool_call>",
		"The `arguments` object MUST include all required parameters and only valid JSON.",
		"Do not invent tool results. Tool results will be provided in <tool_result> tags.",
		"",
	}

	switch parseToolChoice(toolChoice) {
	case "required":
		lines = append(lines, "You MUST call at least one tool.", "")
	default:
		if name := requiredToolName(toolChoice); name != "" {
			lines = append(lines, fmt.Sprintf("You MUST call '%s'.", name), "")
		}
	}

	for _, tool := range tools {
		if tool.Type != "" && tool.Type != "function" {
			continue
		}
		name := tool.Function.Name
		desc := firstSentence(tool.Function.Description)
		schema := parseParameters(tool.Function.Parameters)
		sig := formatSignature(schema)
		line := fmt.Sprintf("- %s(%s)", name, sig)
		if desc != "" {
			line += " — " + desc
		}
		lines = append(lines, line)
	}

	if compact := compactToolSchemas(tools); compact != "" {
		lines = append(lines, "", "<tools>", compact, "</tools>")
	}
	lines = append(lines, "")
	return strings.Join(lines, "\n")
}

type paramSchema struct {
	Type       string                    `json:"type,omitempty"`
	Properties map[string]propertySchema `json:"properties"`
	Required   []string                  `json:"required"`
}

type propertySchema struct {
	Type  string          `json:"type"`
	Enum  []any           `json:"enum,omitempty"`
	Items *propertySchema `json:"items,omitempty"`
}

func parseParameters(raw json.RawMessage) paramSchema {
	if isEmptyJSON(raw) {
		return paramSchema{}
	}
	var schema paramSchema
	_ = json.Unmarshal(raw, &schema)
	return schema
}

func formatSignature(schema paramSchema) string {
	required := map[string]struct{}{}
	for _, name := range schema.Required {
		required[name] = struct{}{}
	}
	names := make([]string, 0, len(schema.Properties))
	for name := range schema.Properties {
		names = append(names, name)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, name := range names {
		info := schema.Properties[name]
		opt := ""
		if _, ok := required[name]; !ok {
			opt = "?"
		}
		typ := info.Type
		if typ == "" {
			typ = "string"
		}
		parts = append(parts, fmt.Sprintf("%s%s: %s", name, opt, typ))
	}
	return strings.Join(parts, ", ")
}

func compactToolSchemas(tools []Tool) string {
	type compactTool struct {
		Name       string      `json:"name"`
		Parameters paramSchema `json:"parameters"`
	}
	out := make([]compactTool, 0, len(tools))
	for _, tool := range tools {
		if tool.Type != "" && tool.Type != "function" {
			continue
		}
		schema := parseParameters(tool.Function.Parameters)
		compactProps := map[string]propertySchema{}
		for name, info := range schema.Properties {
			prop := propertySchema{Type: info.Type}
			if len(info.Enum) > 0 {
				prop.Enum = info.Enum
			}
			if info.Items != nil {
				itemType := info.Items.Type
				if itemType == "" {
					itemType = "object"
				}
				prop.Items = &propertySchema{Type: itemType}
			}
			if prop.Type == "" {
				prop.Type = "string"
			}
			compactProps[name] = prop
		}
		out = append(out, compactTool{
			Name: tool.Function.Name,
			Parameters: paramSchema{
				Type:       "object",
				Properties: compactProps,
				Required:   schema.Required,
			},
		})
	}
	if len(out) == 0 {
		return ""
	}
	b, err := json.Marshal(out)
	if err != nil {
		return ""
	}
	return string(b)
}

func parseToolChoice(raw json.RawMessage) string {
	if isEmptyJSON(raw) {
		return "auto"
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return "auto"
}

func requiredToolName(raw json.RawMessage) string {
	var obj struct {
		Type     string `json:"type"`
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return ""
	}
	if obj.Type != "function" {
		return ""
	}
	return obj.Function.Name
}

func toolSchemaIndex(tools []Tool) map[string]paramSchema {
	index := map[string]paramSchema{}
	for _, tool := range tools {
		if tool.Type != "" && tool.Type != "function" {
			continue
		}
		index[tool.Function.Name] = parseParameters(tool.Function.Parameters)
	}
	return index
}

func normalizeToolArgs(raw any, schema paramSchema) map[string]any {
	args := map[string]any{}
	switch v := raw.(type) {
	case map[string]any:
		args = v
	case string:
		_ = json.Unmarshal([]byte(v), &args)
	}

	for key, val := range args {
		info, ok := schema.Properties[key]
		if !ok {
			continue
		}
		args[key] = coerceType(val, info.Type)
	}
	return args
}

func coerceType(val any, typ string) any {
	if typ == "" {
		typ = "string"
	}
	switch typ {
	case "string":
		if s, ok := val.(string); ok {
			return s
		}
		return fmt.Sprint(val)
	case "integer":
		switch v := val.(type) {
		case int:
			return v
		case float64:
			return int(v)
		case json.Number:
			i, _ := v.Int64()
			return int(i)
		default:
			return 0
		}
	case "number":
		switch v := val.(type) {
		case float64:
			return v
		case int:
			return float64(v)
		case json.Number:
			f, _ := v.Float64()
			return f
		default:
			return 0.0
		}
	case "boolean":
		if b, ok := val.(bool); ok {
			return b
		}
		return val != nil && val != "" && val != 0
	case "array":
		if arr, ok := val.([]any); ok {
			return arr
		}
		return []any{val}
	case "object":
		if obj, ok := val.(map[string]any); ok {
			return obj
		}
		return map[string]any{}
	default:
		return val
	}
}

func extractCallObjects(obj any) []map[string]any {
	switch v := obj.(type) {
	case []any:
		out := make([]map[string]any, 0, len(v))
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	case map[string]any:
		if calls, ok := v["tool_calls"].([]any); ok {
			return extractCallObjects(calls)
		}
		return []map[string]any{v}
	default:
		return nil
	}
}

func ParseToolCalls(content string, tools []Tool, newID func() string) (string, []ToolCall) {
	if len(tools) == 0 {
		return stripToolCallXML(content), nil
	}

	matches := toolCallPattern.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		// Still strip unclosed / leftover <tool_call> so clients never see XML.
		return stripToolCallXML(content), nil
	}

	allowed := toolSchemaIndex(tools)
	calls := make([]ToolCall, 0, len(matches))
	for _, match := range matches {
		raw := strings.TrimSpace(match[1])
		var parsed any
		if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
			continue
		}
		for _, item := range extractCallObjects(parsed) {
			name := callName(item)
			if name == "" {
				continue
			}
			schema, ok := allowed[name]
			if !ok {
				continue
			}
			args := callArguments(item)
			normalized := normalizeToolArgs(args, schema)
			argsJSON, err := json.Marshal(normalized)
			if err != nil || string(argsJSON) == "null" {
				argsJSON = []byte("{}")
			}
			calls = append(calls, ToolCall{
				ID:   newID(),
				Type: "function",
				Function: ToolCallFunction{
					Name:      name,
					Arguments: string(argsJSON),
				},
			})
		}
	}

	return stripToolCallXML(content), calls
}

func stripToolCallXML(s string) string {
	out := toolCallPattern.ReplaceAllString(s, "")
	out = unclosedToolCallPattern.ReplaceAllString(out, "")
	return strings.TrimSpace(out)
}

func callName(item map[string]any) string {
	if name, ok := item["name"].(string); ok && name != "" {
		return name
	}
	if name, ok := item["tool"].(string); ok && name != "" {
		return name
	}
	if name, ok := item["tool_name"].(string); ok && name != "" {
		return name
	}
	if fn, ok := item["function"].(map[string]any); ok {
		if name, ok := fn["name"].(string); ok {
			return name
		}
	}
	return ""
}

func callArguments(item map[string]any) any {
	for _, key := range []string{"arguments", "parameters", "args", "tool_input", "input"} {
		if val, ok := item[key]; ok {
			return val
		}
	}
	if fn, ok := item["function"].(map[string]any); ok {
		if val, ok := fn["arguments"]; ok {
			return val
		}
	}
	return map[string]any{}
}
