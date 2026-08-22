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

1. Client calls OpenAI-compatible HTTP (`GET /`, `GET /health`, `GET /healthz`, `GET /v1/models`, `POST /v1/chat/completions`, `POST /v1/completions`), native `POST /api/chat`, Anthropic `POST /v1/messages`, or Gemini `generateContent` / `streamGenerateContent`. Path aliases (`/models`, `/chat/completions`, `/completions`, `/api/health`, `/api/v1-models`, `/api/v1-chat-completions`, `/api/v1-completions`) map onto the OpenAI handlers.
2. Gateway merges system messages, filters OpenCode orchestration tools, injects `<tool_call>` instructions when tools remain (`tool_choice: none` skips that), maps sampling fields into ChatJimmy `chatOptions`, and optionally extracts a single image attachment. Oversized prompts drop the `<tools>` JSON block before the 28K hard slice; `<tool_result>` bodies are capped at 8K.
3. Upstream talk goes to ChatJimmy over HTTPS (`https://chatjimmy.ai/api/chat` by default) with browser-like `Origin` / `Referer` / `User-Agent` headers. Network / 408 / 429 / 5xx responses retry with backoff; an empty body is retried once on top of that.
4. The upstream response is parsed: `<|stats|>` / `<stats>` and thinking/reasoning tags are stripped, token usage is extracted, assistant text is kept, and `<tool_call>` XML is converted into OpenAI `tool_calls` only for allow-listed tools from this request (unknown names dropped; raw XML stripped even when tools are empty / `tool_choice: none`). Anthropic and Gemini adapters wrap the same completion.
5. Non-streaming clients get a single JSON completion; streaming clients receive buffered SSE in the requested protocol shape (chat streams may include a final usage chunk when `stream_options.include_usage` is set). Time-to-first-token equals full generation time because ChatJimmy is fully read before any chunk is written.

</details>

<details><summary><strong>Features</strong></summary>

### Features

- **OpenAI-compatible API:** `/v1/models`, `/v1/chat/completions`, and legacy `/v1/completions` for text chat, tool calls, and streaming. Health is `GET /`, `GET /health`, and `GET /healthz` (`{"status":"ok"}`). Unprefixed aliases `/models`, `/chat/completions`, and `/completions` are accepted.
- **Native ChatJimmy passthrough:** `POST /api/chat` forwards `{messages, chatOptions, attachment}` without OpenAI translation.
- **Anthropic / Gemini HTTP:** `POST /v1/messages` and Gemini `POST /v1beta/models/{model}:generateContent` (plus `:streamGenerateContent`) reuse the same XML tool layer. Stream flags still buffer until ChatJimmy actually streams.
- **ChatJimmy upstream:** Proxies to ChatJimmy's Llama 3.1 8B chat endpoint (`https://chatjimmy.ai/api/chat` by default). URL, timeout, and optional upstream Bearer (`CHATJIMMY_API_KEY`) are configurable.
- **Request translation:** Maps OpenAI `system` / `user` / `assistant` / `tool` messages into ChatJimmy's `{messages, chatOptions, attachment}` payload, including system-prompt merge and assistant/tool round-trips.
- **Sampling passthrough:** Forwards `temperature` (OpenAI 0–2 scaled to ChatJimmy 0–1), `top_p`, `top_k` / `topK` (default 8), `max_tokens`, and `stop` / `stopSequences`. Native `chatOptions` on the request body are accepted as a fallback.
- **Model aliases:** `/v1/models` lists `llama3.1-8B` plus common OpenAI / Anthropic / Gemini ids. All of them map to the same upstream model. Extra advertised ids come from `CHATJIMMY_MODELS`.
- **Tool XML emulation:** Injects Llama-friendly tool instructions and parses `<tool_call>` blocks back into OpenAI `tool_calls` for allow-listed tools only (`tool_choice` of `auto`, `none`, `required`, or a named function). Unknown names are dropped; raw XML is stripped even when tools are empty or tags are unclosed. `tool_choice: none` skips schema inject and XML parse.
- **OpenCode tool filtering:** Strips high-level orchestration tools (`webfetch`, `todowrite`, `skill`, `question`, `task`) before they reach the model so smaller prompts stay on file/shell/search tools.
- **System prompt safeguard:** Truncates oversized system prompts at 28K characters to avoid ChatJimmy's silent empty-response limit (~30K). Before that hard slice, `<tools>…</tools>` JSON is dropped; `<tool_result>` bodies are capped at 8K characters.
- **Image attachments:** Forwards one OpenAI `image_url` data-URL or Gemini-style `inlineData` part as ChatJimmy `attachment`.
- **Stats mapping:** Strips `<|stats|>` / `<stats>` JSON and thinking/reasoning tags (including unclosed openers), fills OpenAI `usage` from `prefill_tokens` / `decode_tokens` / `total_tokens`, and echoes raw metrics as `chatjimmy_stats`.
- **Optional gateway auth:** When `API_KEY` is set, chat routes require `Authorization: Bearer …`, `x-api-key`, or `x-goog-api-key` (`OPENAI_API_KEY` is accepted as an alias).
- **CORS:** OPTIONS and JSON/SSE responses send `Access-Control-Allow-Origin` from `ALLOWED_ORIGIN` (default `*`). A non-wildcard origin also sets `Vary: Origin`. Allow-headers include `x-api-key`.
- **Buffered streaming:** Upstream responses are buffered first, then emitted as SSE so clients still get protocol-shaped stream chunks (including tool-call deltas after the full XML is parsed). Chat and legacy completions streams may append a final usage chunk when `stream_options.include_usage` is set.
- **Retries:** Network / 408 / 429 / 5xx retry up to 3 extra times with 1s/2s/4s backoff (cap 10s), honoring `Retry-After`. An empty ChatJimmy body is retried once without consuming that budget.
- **Access logs:** Every request logs method, client path (aliases preserved), status, duration in ms, and remote addr. Chat requests also log a light `kind`/`model`/`upstream` summary.
- **Verbose logs:** `-b` / `VERBOSE` adds truncation/compaction flags and per-chat token counts on the chat summary line.

</details>

<details><summary><strong>Out of scope</strong></summary>

### Out of scope

- **Client request rate limiting:** Not implemented; use a reverse proxy if you need it.
- **HTTPS / TLS termination:** Listen plain HTTP; terminate TLS in front (Caddy, etc.).
- **Account pools / OAuth:** Single ChatJimmy upstream endpoint; no session store, token refresh, or multi-account load balancing.
- **Native function calling:** ChatJimmy has no native tools API. Tool use is emulated in the prompt/response layer only.
- **True upstream streaming:** Time-to-first-token equals full generation time because the gateway buffers ChatJimmy's response before emitting SSE.
- **Embeddings / extra OpenAI APIs:** No `/v1/embeddings`, images, audio, or files endpoints.
- **Multiple upstream models:** ChatJimmy currently exposes Llama 3.1 8B only. Advertised aliases and extra `CHATJIMMY_MODELS` ids all forward to that service.
- **Model quality expectations:** Llama 3.1 8B on ChatJimmy is aggressively quantized (3-bit/6-bit) on a small context window, so quality is below typical GPU baselines. It is built for speed, not deep reasoning.

</details>

<details><summary><strong>Future</strong></summary>

### Future

Shipped items from the previous list (CORS, upstream config, retries, native `/api/chat`, `tool_choice: none`, Anthropic/Gemini HTTP, tool-JSON compaction, verbose latency logs, extra model ids) are in [Features](#features). Still open if ChatJimmy stays a single chat endpoint:

- **True SSE:** Emit tokens as they arrive if ChatJimmy ever streams instead of returning one body. Not implemented today — SSE is always post-buffer.
- **Prompt summarization:** Compact old turns with an LLM when dropping `<tools>` JSON is not enough.
- **Metrics:** Optional Prometheus / structured metrics beyond verbose logs.
- **New ChatJimmy models:** Advertise extra ids when Taalas ships the planned mid-size reasoning model (`CHATJIMMY_MODELS` already accepts extras).

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

The gateway exposes OpenAI-compatible endpoints plus native ChatJimmy, Anthropic, and Gemini HTTP:

| Method | Path | Description |
| ------ | ---- | ----------- |
| `GET`  | `/`, `/health`, `/healthz` | Health check (`{"status":"ok"}`) |
| `GET`  | `/v1/models` | List advertised model ids |
| `POST` | `/v1/chat/completions` | OpenAI chat completion (JSON or buffered SSE) |
| `POST` | `/v1/completions` | OpenAI legacy text completion (JSON or buffered SSE) |
| `POST` | `/api/chat` | Native ChatJimmy passthrough (`{messages, chatOptions, attachment}`) |
| `POST` | `/v1/messages` | Anthropic Messages (JSON or buffered SSE) |
| `POST` | `/v1beta/models/{model}:generateContent` | Gemini generate (JSON) |
| `POST` | `/v1beta/models/{model}:streamGenerateContent` | Gemini stream (buffered SSE) |

Aliases: `/api` → `/`, `/api/health` → `/health`, `/models` → `/v1/models`, `/chat/completions` → `/v1/chat/completions`, `/completions` → `/v1/completions`, `/api/v1-models` → `/v1/models`, `/api/v1-chat-completions` → `/v1/chat/completions`, `/api/v1-completions` → `/v1/completions`.

`POST /api/chat` forwards the ChatJimmy payload as-is and returns the raw upstream body as `text/plain`. Anthropic and Gemini wrap the same XML tool layer as OpenAI; stream flags still buffer until ChatJimmy actually streams.

**Request body** for `POST /v1/chat/completions`:

| Field         | Type             | Default       | Description                                                                        |
| ------------- | ---------------- | ------------- | ---------------------------------------------------------------------------------- |
| `model`       | string           | `llama3.1-8B` | Model id (aliases are rewritten upstream)                                          |
| `messages`    | array            | required      | `{role, content}` messages (`system`, `user`, `assistant`, `tool`)                 |
| `stream`         | boolean          | `false`       | Enable SSE streaming                                                               |
| `stream_options` | object           | _(omit)_      | `{ "include_usage": true }` appends a final usage chunk on buffered chat SSE       |
| `tools`          | array            | `[]`          | OpenAI-format tool definitions (filtered, see [Features](#features))               |
| `tool_choice`    | string \| object | `"auto"`      | `"auto"`, `"none"`, `"required"`, or `{"type":"function","function":{"name":"…"}}` |
| `temperature`    | number           | _(omit)_      | OpenAI 0–2, scaled to ChatJimmy 0–1                                                |
| `top_p`          | number           | _(omit)_      | Nucleus sampling (0–1)                                                             |
| `top_k`          | integer          | `8`           | ChatJimmy `topK` (`topK` camelCase is also accepted)                               |
| `max_tokens`     | integer          | _(omit)_      | ChatJimmy `maxTokens`                                                              |
| `stop`           | string \| array  | _(omit)_      | Mapped to `stopSequences`                                                          |

Example requests:

```sh
curl -s http://127.0.0.1:8080/v1/models

curl -s http://127.0.0.1:8080/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -d '{"model":"llama3.1-8B","messages":[{"role":"user","content":"Hello"}]}'

curl -s http://127.0.0.1:8080/v1/completions \
  -H 'Content-Type: application/json' \
  -d '{"model":"llama3.1-8B","prompt":"Hello","max_tokens":32}'
```

When `stream: true`, responses use SSE chunks in the standard `chat.completion.chunk` / `text_completion` format, then `data: [DONE]`. Chat and legacy completions streams may include a final usage chunk when `stream_options.include_usage` is set.

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

| Provider       | Client id                    | Upstream      |
| -------------- | ---------------------------- | ------------- |
| ChatJimmy      | `llama3.1-8B`                | `llama3.1-8B` |
| OpenAI         | `gpt-4o`                     | `llama3.1-8B` |
| OpenAI         | `gpt-4o-mini`                | `llama3.1-8B` |
| OpenAI         | `gpt-4-turbo`                | `llama3.1-8B` |
| OpenAI         | `gpt-4`                      | `llama3.1-8B` |
| OpenAI         | `gpt-3.5-turbo`              | `llama3.1-8B` |
| OpenAI         | `gpt-3.5-turbo-instruct`     | `llama3.1-8B` |
| Anthropic      | `claude-opus-4-5-20251101`   | `llama3.1-8B` |
| Anthropic      | `claude-sonnet-4-5-20250929` | `llama3.1-8B` |
| Anthropic      | `claude-3-5-haiku-20241022`  | `llama3.1-8B` |
| Anthropic      | `claude-3-opus-20240229`     | `llama3.1-8B` |
| Anthropic      | `claude-3-sonnet-20240229`   | `llama3.1-8B` |
| Anthropic      | `claude-3-haiku-20240307`    | `llama3.1-8B` |
| Google Gemini  | `gemini-1.5-pro`             | `llama3.1-8B` |
| Google Gemini  | `gemini-1.5-flash`           | `llama3.1-8B` |
| Google Gemini  | `gemini-1.0-pro`             | `llama3.1-8B` |
| Google Gemini  | `gemini-1.0-ultra`           | `llama3.1-8B` |

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
- `API_KEY` or `--api-key`: optional gateway key for chat routes. Clients may send `Authorization: Bearer …`, `x-api-key`, or `x-goog-api-key`. `OPENAI_API_KEY` is used when `API_KEY` is empty. Off by default.
- `VERBOSE` or `-b` / `--verbose`: add truncation/compaction flags and token counts to the always-on chat summary line. Access logs (including client path + ms duration) are always on. Defaults to `false`.
- `ALLOWED_ORIGIN` or `--allowed-origin`: CORS `Access-Control-Allow-Origin`. Defaults to `*`. A non-wildcard value also sets `Vary: Origin`.
- `CHATJIMMY_URL` or `--chatjimmy-url`: upstream chat URL. Defaults to `https://chatjimmy.ai/api/chat`. Must be `http` or `https`.
- `CHATJIMMY_TIMEOUT` or `--chatjimmy-timeout`: upstream timeout in seconds (1–300). Defaults to `120`.
- `CHATJIMMY_API_KEY` or `--chatjimmy-api-key`: optional Bearer key sent upstream. Separate from gateway `API_KEY`.
- `CHATJIMMY_MODEL` or `--chatjimmy-model`: default advertised / native model id. Defaults to `llama3.1-8B`.
- `CHATJIMMY_MODELS` or `--chatjimmy-models`: extra advertised model ids, comma-separated. Aliases still map to ChatJimmy's single Llama 3.1 8B.

Example `.env`:

```bash
PORT=8080
API_KEY=your-secret
ALLOWED_ORIGIN=*
CHATJIMMY_URL=https://chatjimmy.ai/api/chat
CHATJIMMY_TIMEOUT=120
```

Default `topK` is `8`. Production retries are 3 extra attempts with 1s/2s/4s backoff (cap 10s). Run `jimmy-gateway -h` for CLI flags.

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

OpenAI-compat routing, validation, and stream `include_usage` behavior are covered by `go test` under `internal/service` (no separate `make smoke`).

Auto-reload with Air:

```sh
make dev
```

Run in Docker with Air (detached; uses `compose.yml`; host `PORT` from `.env` maps to container `8080`):

```sh
make docker/dev
docker compose logs -f local
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