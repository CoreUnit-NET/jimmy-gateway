package service

import "strings"

var pathAliases = map[string]string{
	"/api":                     "/",
	"/api/health":              "/health",
	"/healthz":                 "/health",
	"/api/v1-models":           "/v1/models",
	"/models":                  "/v1/models",
	"/api/v1-chat-completions": "/v1/chat/completions",
	"/chat/completions":        "/v1/chat/completions",
	"/api/v1-completions":      "/v1/completions",
	"/completions":             "/v1/completions",
}

func resolvePath(pathname string) string {
	return normalizePath(mapAliasPath(normalizePath(pathname)))
}

func parseGeminiPath(pathname string) (model string, stream bool, ok bool) {
	const prefix = "/v1beta/models/"
	if !strings.HasPrefix(pathname, prefix) {
		return "", false, false
	}
	rest := strings.TrimPrefix(pathname, prefix)
	switch {
	case strings.HasSuffix(rest, ":streamGenerateContent"):
		model = strings.TrimSuffix(rest, ":streamGenerateContent")
		return model, true, model != ""
	case strings.HasSuffix(rest, ":generateContent"):
		model = strings.TrimSuffix(rest, ":generateContent")
		return model, false, model != ""
	default:
		return "", false, false
	}
}

func mapAliasPath(pathname string) string {
	if alias, ok := pathAliases[pathname]; ok {
		return alias
	}
	return pathname
}

func normalizePath(pathname string) string {
	if pathname == "" {
		return "/"
	}
	if len(pathname) > 1 {
		pathname = strings.TrimRight(pathname, "/")
	}
	return pathname
}
