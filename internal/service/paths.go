package service

import "strings"

var pathAliases = map[string]string{
	"/api":                     "/",
	"/api/health":              "/health",
	"/api/v1-models":           "/v1/models",
	"/api/v1-chat-completions": "/v1/chat/completions",
}

func resolvePath(pathname string) string {
	return normalizePath(mapAliasPath(normalizePath(pathname)))
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
