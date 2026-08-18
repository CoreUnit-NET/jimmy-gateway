<div align="center">

# 💬🧑‍💻🔥 jimmy-gateway

![AGPL-3.0](https://img.shields.io/badge/license-AGPL--3.0-blue.svg)
![CI/CD](https://github.com/CoreUnit-NET/jimmy-gateway/actions/workflows/go-bin-release.yml/badge.svg)
![CI/CD](https://github.com/CoreUnit-NET/jimmy-gateway/actions/workflows/go-test-build.yml/badge.svg)  
![](https://img.shields.io/badge/dynamic/json?color=green&label=watchers&query=watchers&suffix=x&url=https%3A%2F%2Fapi.github.com%2Frepos%2FCoreUnit-NET%2Fjimmy-gateway)
![](https://img.shields.io/badge/dynamic/json?color=yellow&label=stars&query=stargazers_count&suffix=x&url=https%3A%2F%2Fapi.github.com%2Frepos%2FCoreUnit-NET%2Fjimmy-gateway)
![](https://img.shields.io/badge/dynamic/json?color=navy&label=forks&query=forks&suffix=x&url=https%3A%2F%2Fapi.github.com%2Frepos%2FCoreUnit-NET%2Fjimmy-gateway)

</div>

_Do you want OpenAI-compatible clients to talk to [ChatJimmy](https://chatjimmy.ai)?_
`jimmy-gateway` is an OpenAI-compatible HTTP proxy for ChatJimmy that solves that for you.

## About

`jimmy-gateway` is a Go proxy for setups where agents and tools need [ChatJimmy](https://chatjimmy.ai)'s hardware-accelerated Llama 3.1 8B—powered by [Taalas](https://taalas.com) custom silicon—through a normal OpenAI-shaped HTTP API.

It translates `/v1/chat/completions` (and related OpenAI-like routes) into ChatJimmy's chat payload, forwards them to `chatjimmy.ai`, and maps the response back into OpenAI chat completion format—including streamed SSE when requested.

Put it on localhost or a private network next to your agents (OpenCode, custom scripts, other OpenAI SDK clients). Terminate TLS and extra edge auth in front if you need them—this process stays a plain HTTP gateway.

ChatJimmy currently exposes a single public chat endpoint and no native function-calling API. The gateway emulates OpenAI tools in the prompt, advertises common provider model ids, and keeps the 28K system-prompt ceiling from silently emptying replies.

<details><summary><strong>How it works</strong></summary>

### How it works

1. Client calls OpenAI-compatible HTTP (`GET /`, `GET /health`, `GET /v1/models`, `POST /v1/chat/completions`, stream or non-stream). Path aliases (`/api/health`, `/api/v1-models`, `/api/v1-chat-completions`) map onto the same handlers.
2. Gateway merges system messages, filters OpenCode orchestration tools, injects `<tool_call>` instructions when tools remain, maps sampling fields into ChatJimmy `chatOptions`, and optionally extracts a single image attachment.
3. Upstream talk goes to ChatJimmy over HTTPS (`https://chatjimmy.ai/api/chat` by default) with browser-like `Origin` / `Referer` / `User-Agent` headers. An empty body is retried once.
4. The upstream response is parsed: `<|stats|>` / `<stats>` tags are stripped, token usage is extracted, assistant text is kept, and `<tool_call>` XML is converted into OpenAI `tool_calls` when tools were offered.
5. Non-streaming clients get a single JSON completion; streaming clients receive buffered SSE chunks in OpenAI `chat.completion.chunk` shape. Time-to-first-token equals full generation time because ChatJimmy is fully read before any chunk is written.

</details>

<details><summary><strong>Features</strong></summary>

### Features

- **OpenAI-compatible API:** `/v1/models` and `/v1/chat/completions` for text chat, tool calls, and streaming. Health is `GET /` and `GET /health` (`{"status":"ok"}`).
- **ChatJimmy upstream:** Proxies to ChatJimmy's Llama 3.1 8B chat endpoint (`https://chatjimmy.ai/api/chat`). No upstream API key is required today.
- **Request translation:** Maps OpenAI `system` / `user` / `assistant` / `tool` messages into ChatJimmy's `{messages, chatOptions, attachment}` payload, including system-prompt merge and assistant/tool round-trips.
- **Sampling passthrough:** Forwards `temperature` (OpenAI 0–2 scaled to ChatJimmy 0–1), `top_p`, `top_k` / `topK` (default 8), `max_tokens`, and `stop` / `stopSequences`. Native `chatOptions` on the request body are accepted as a fallback.
- **Model aliases:** `/v1/models` lists `llama3.1-8B` plus common OpenAI / Anthropic / Gemini ids. All of them map to the same upstream model.
- **Tool XML emulation:** Injects Llama-friendly tool instructions and parses `<tool_call>` blocks back into OpenAI `tool_calls`, including `tool_choice` of `auto`, `none`, `required`, or a named function.
- **OpenCode tool filtering:** Strips high-level orchestration tools (`webfetch`, `todowrite`, `skill`, `question`, `task`) before they reach the model so smaller prompts stay on file/shell/search tools.
- **System prompt safeguard:** Truncates oversized system prompts at 28K characters to avoid ChatJimmy's silent empty-response limit (~30K).
- **Image attachments:** Forwards one OpenAI `image_url` data-URL or Gemini-style `inlineData` part as ChatJimmy `attachment`.
- **Stats mapping:** Strips `<|stats|>` / `<stats>` JSON, fills OpenAI `usage` from `prefill_tokens` / `decode_tokens` / `total_tokens`, and echoes raw metrics as `chatjimmy_stats`.
- **Optional gateway auth:** When `API_KEY` is set, `/v1/chat/completions` requires `Authorization: Bearer …` (`OPENAI_API_KEY` is accepted as an alias).
- **CORS:** OPTIONS and JSON/SSE responses send `Access-Control-Allow-Origin: *` plus allow-methods/headers for browser or local tooling.
- **Buffered streaming:** Upstream responses are buffered first, then emitted as SSE so clients still get OpenAI-style stream chunks (including tool-call deltas after the full XML is parsed).
- **Empty-body retry:** If ChatJimmy returns an empty body, the gateway retries the same payload once.

</details>

<details><summary><strong>Out of scope</strong></summary>

### Out of scope

- **Client request rate limiting:** Not implemented; use a reverse proxy if you need it.
- **HTTPS / TLS termination:** Listen plain HTTP; terminate TLS in front (Caddy, etc.).
- **Account pools / OAuth:** Single ChatJimmy upstream endpoint; no session store, token refresh, or multi-account load balancing.
- **Native function calling:** ChatJimmy has no native tools API. Tool use is emulated in the prompt/response layer only.
- **True upstream streaming:** Time-to-first-token equals full generation time because the gateway buffers ChatJimmy's response before emitting SSE.
- **Embeddings / extra OpenAI APIs:** No `/v1/embeddings`, `/v1/completions`, images, audio, or files endpoints.
- **Anthropic / Gemini HTTP:** Model _names_ are aliased on the OpenAI routes. There is no `POST /v1/messages` or Gemini `generateContent` server.
- **Multiple upstream models:** ChatJimmy currently exposes Llama 3.1 8B only. Advertised aliases all forward to that service.
- **Model quality expectations:** Llama 3.1 8B on ChatJimmy is aggressively quantized (3-bit/6-bit) on a small context window, so quality is below typical GPU baselines. It is built for speed, not deep reasoning.

</details>

<details><summary><strong>Future</strong></summary>

### Future

These are not implemented. They are the next gateway-side improvements if ChatJimmy stays a single chat endpoint.

- **Configurable CORS origin:** `ALLOWED_ORIGIN` / `--allowed-origin` instead of hard-coded `*`.
- **Configurable upstream:** URL, timeout, and optional upstream auth header (today the chat URL and 120s timeout are built in).
- **Retry on 5xx / 429:** Exponential backoff in addition to the empty-body retry.
- **Native passthrough:** `POST /api/chat` that forwards ChatJimmy's own `{messages, chatOptions, attachment}` body without OpenAI translation.
- **Skip tools on `tool_choice: none`:** Do not inject the tool schema when the client forbids calls.
- **Anthropic Messages / Gemini generateContent:** HTTP adapters that reuse the existing XML tool layer (not a second translator).
- **Prompt compaction:** Summarize or drop old turns before the 28K system-prompt hard truncate.
- **Observability:** Structured request logs, latency, and optional metrics.
- **True SSE:** Emit tokens as they arrive if ChatJimmy ever streams instead of returning one body.
- **New model ids:** Advertise extra ChatJimmy models when Taalas ships the planned mid-size reasoning model; keep aliasing until then.

Not planned here (needs ChatJimmy itself, or another process in front): native function calling, embeddings, TLS, client rate limits, multi-account OAuth.

</details>

<details><summary><strong>Usage</strong></summary>

## Usage

Show help messages:

```sh
jimmy-gateway
```

Running without a subcommand starts the HTTP server (same as `jimmy-gateway serve`).

Point clients at `http://<host>:<port>/v1` (default `http://0.0.0.0:8080/v1`).

### version

Print build version information:

```sh
jimmy-gateway version
jimmy-gateway -v
```

### serve

Start the OpenAI-compatible proxy:

```sh
jimmy-gateway serve
```

### API

The gateway exposes standard OpenAI-compatible endpoints:

| Method | Path                   | Description                      |
| ------ | ---------------------- | -------------------------------- |
| `GET`  | `/`, `/health`         | Health check (`{"status":"ok"}`) |
| `GET`  | `/v1/models`           | List advertised model ids        |
| `POST` | `/v1/chat/completions` | Chat completion (JSON or SSE)    |

Aliases: `/api` → `/`, `/api/health` → `/health`, `/api/v1-models` → `/v1/models`, `/api/v1-chat-completions` → `/v1/chat/completions`.

**Request body** for `POST /v1/chat/completions`:

| Field         | Type             | Default       | Description                                                                        |
| ------------- | ---------------- | ------------- | ---------------------------------------------------------------------------------- |
| `model`       | string           | `llama3.1-8B` | Model id (aliases are rewritten upstream)                                          |
| `messages`    | array            | required      | `{role, content}` messages (`system`, `user`, `assistant`, `tool`)                 |
| `stream`      | boolean          | `false`       | Enable SSE streaming                                                               |
| `tools`       | array            | `[]`          | OpenAI-format tool definitions (filtered, see [Features](#features))               |
| `tool_choice` | string \| object | `"auto"`      | `"auto"`, `"none"`, `"required"`, or `{"type":"function","function":{"name":"…"}}` |
| `temperature` | number           | _(omit)_      | OpenAI 0–2, scaled to ChatJimmy 0–1                                                |
| `top_p`       | number           | _(omit)_      | Nucleus sampling (0–1)                                                             |
| `top_k`       | integer          | `8`           | ChatJimmy `topK` (`topK` camelCase is also accepted)                               |
| `max_tokens`  | integer          | _(omit)_      | ChatJimmy `maxTokens`                                                              |
| `stop`        | string \| array  | _(omit)_      | Mapped to `stopSequences`                                                          |

Example requests:

```sh
curl -s http://127.0.0.1:8080/v1/models

curl -s http://127.0.0.1:8080/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -d '{"model":"llama3.1-8B","messages":[{"role":"user","content":"Hello"}]}'
```

When `stream: true`, responses use SSE chunks in the standard `chat.completion.chunk` format, then `data: [DONE]`.

Non-stream JSON also includes OpenAI `usage` and optional `chatjimmy_stats` from the upstream `<|stats|>` block. `finish_reason` is `tool_calls` when XML tools were parsed, otherwise `stop` or `length` from stats.

### Tool calling

ChatJimmy does not expose native tools. The gateway emulates OpenAI function calling:

1. Client sends `tools` (and optional `tool_choice`).
2. OpenCode orchestration tools are dropped; remaining schemas are appended to the system prompt as `<tool_call>` XML instructions.
3. The model replies with one or more `<tool_call>{"name","arguments"}</tool_call>` blocks.
4. The gateway returns OpenAI `tool_calls` and `finish_reason: tool_calls`.
5. The client runs the tools and posts `role: tool` messages.
6. Those results are rewritten as user `<tool_result>` blocks and the chat continues.

This is prompt/response translation only. Do not expect native tool APIs from ChatJimmy.

### Models

`GET /v1/models` lists `llama3.1-8B` first, then aliases. Unknown client model names are forwarded as `selectedModel` unchanged.

| Client id                                                                                                                                                              | Upstream      |
| ---------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------- |
| `llama3.1-8B`                                                                                                                                                          | `llama3.1-8B` |
| `gpt-4o`, `gpt-4o-mini`, `gpt-4-turbo`, `gpt-4`, `gpt-3.5-turbo`, `gpt-3.5-turbo-instruct`                                                                             | `llama3.1-8B` |
| `claude-opus-4-5-20251101`, `claude-sonnet-4-5-20250929`, `claude-3-5-haiku-20241022`, `claude-3-opus-20240229`, `claude-3-sonnet-20240229`, `claude-3-haiku-20240307` | `llama3.1-8B` |
| `gemini-1.5-pro`, `gemini-1.5-flash`, `gemini-1.0-pro`, `gemini-1.0-ultra`                                                                                             | `llama3.1-8B` |

### OpenCode setup

Point OpenCode at the gateway base URL. Example `opencode.json`:

```json
{
  "$schema": "https://opencode.ai/config.json",
  "provider": {
    "chatjimmy": {
      "npm": "@ai-sdk/openai-compatible",
      "name": "ChatJimmy",
      "options": {
        "baseURL": "http://localhost:8080/v1"
      },
      "models": {
        "llama3.1-8B": {
          "name": "Llama 3.1 8B (ChatJimmy)"
        }
      }
    }
  }
}
```

If you set `API_KEY` on the gateway, add the same value as the provider API key in OpenCode.

</details>

<details><summary><strong>Configuration</strong></summary>

## Configuration

CLI flags and environment variables can both be used. **Flags override env values.**

A `.env` file in the working directory is loaded at startup when present (missing file is ignored). Defaults work out of the box—no setup required to proxy ChatJimmy.

`serve` / runtime flags and environment variables:

- `HOST` or `--host`: bind host, defaults to `0.0.0.0`
- `PORT` or `-p` / `--port`: bind port, defaults to `8080`
- `API_KEY` or `--api-key`: optional Bearer key for `/v1/chat/completions`. `OPENAI_API_KEY` is used when `API_KEY` is empty. Off by default.
- `VERBOSE` or `-b` / `--verbose`: log method and path, defaults to `false`

Example `.env`:

```bash
PORT=8080
API_KEY=your-secret
```

ChatJimmy upstream URL, model, and default `topK` are built in: `https://chatjimmy.ai/api/chat`, model `llama3.1-8B`, `topK` 8. CORS origin is currently `*`. Run `jimmy-gateway -h` for CLI flags.

</details>

<details><summary><strong>User Guide</strong></summary>

# User Guide

## Requirements

Linux- or macos-like systems with `go` or `wget & tar` installed.

## Getting Started

Start the latest repo version directly without leaving stuff in the current working dir:

```sh
go run github.com/CoreUnit-NET/jimmy-gateway@latest
```

## Quick help

```sh
go run github.com/CoreUnit-NET/jimmy-gateway@latest -h
```

## Install via go

###### _For this section go is required, check out the [install go guide](#install-go)._

```sh
go install github.com/CoreUnit-NET/jimmy-gateway@latest
```

## Install via wget

```sh
export CUSTOM_BIN_DIR="/usr/local/bin" # <- change if needed
export CUSTOM_VERSION="" # <- set latest version here

rm -rf $CUSTOM_BIN_DIR/jimmy-gateway
wget https://github.com/CoreUnit-NET/jimmy-gateway/releases/download/v$CUSTOM_VERSION/jimmy-gateway-v$CUSTOM_VERSION-linux-amd64.tar.gz -O /tmp/jimmy-gateway.tar.gz
tar -xzvf /tmp/jimmy-gateway.tar.gz -C $CUSTOM_BIN_DIR/ jimmy-gateway
rm /tmp/jimmy-gateway.tar.gz
```

# Build

## Build requirements

To build, you need to install go.
The required go version is in the `go.mod` file.

## Build Instructions

###### _For this section go is required, check out the [install go guide](#install-go)._

Clone the repo:

```sh
git clone https://github.com/CoreUnit-NET/jimmy-gateway.git
cd jimmy-gateway
```

Build the jimmy-gateway binary from source code:

```sh
make build
./bin
```

</details>

<details><summary><strong>Development</strong></summary>

# Development

###### _For this section go is required, check out the [install go guide](#install-go)._

Run tests and build:

```sh
make test
make build
```

Auto-reload with Air:

```sh
make dev
```

Run in Docker with Air (uses `compose.yml`):

```sh
make docker/dev
```

## Install go

The required go version for this project is in the `go.mod` file.

To install and update go, I can recommend the following repo:

```sh
git clone git@github.com:udhos/update-golang.git golang-updater
cd golang-updater
sudo ./update-golang.sh
```

</details>

<div align="center">

# 🤝 Contributing

Contributions to this project are welcome!  
Follow the [CONTRIBUTING.md](CONTRIBUTING.md) for more infos.

# ⚠️ Disclaimer

This project is provided without warranties.

# 📜 License

Licensed under the [GNU Affero General Public License v3](LICENSE).

<a href="https://discord.coreunit.net">
    <img alt="CoreUnit.NET Discord Banner" src="https://discord.com/api/guilds/422136748294930443/widget.png?style=banner2">
</a>

</div>
