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

<details><summary><strong>How it works</strong></summary>

### How it works

1. Client calls OpenAI-compatible HTTP (`GET /`, `GET /health`, `GET /v1/models`, `POST /v1/chat/completions`, stream or non-stream).
2. Gateway merges system messages, filters OpenCode orchestration tools, injects tool instructions when needed, and builds the ChatJimmy request payload.
3. Upstream talk goes to ChatJimmy over HTTPS (`https://chatjimmy.ai/api/chat` by default) with browser-like headers.
4. The upstream response is parsed: `<|stats|>` / `<stats>` tags are stripped, token usage is extracted, assistant text is kept, and `<tool_call>` XML is converted into OpenAI `tool_calls` when tools were offered.
5. Non-streaming clients get a single JSON completion; streaming clients receive buffered SSE chunks in OpenAI `chat.completion.chunk` shape.

</details>

<details><summary><strong>Features</strong></summary>

### Features

- **OpenAI-compatible API:** `/v1/models` and `/v1/chat/completions` for text chat, tool calls, and streaming.
- **ChatJimmy upstream:** Proxies to ChatJimmy's Llama 3.1 8B chat endpoint.
- **Request translation:** Maps OpenAI roles/messages into ChatJimmy's chat format, including system-prompt merge and assistant/tool round-trip handling.
- **Tool XML emulation:** Injects Llama-friendly tool instructions and parses `<tool_call>` blocks back into OpenAI `tool_calls`.
- **OpenCode tool filtering:** Strips high-level orchestration tools (`webfetch`, `todowrite`, `skill`, `question`, `task`) before they reach the model.
- **System prompt safeguard:** Truncates oversized system prompts at 28K characters to avoid ChatJimmy's silent empty-response limit (~30K).
- **Optional gateway auth:** When `API_KEY` is set, `/v1/chat/completions` requires `Authorization: Bearer …` (ChatJimmy itself needs no upstream key today).
- **CORS:** Configurable `Access-Control-Allow-Origin` for browser or local tooling.
- **Health checks:** `GET /` and `GET /health` return `{"status":"ok"}`.
- **Buffered streaming:** Upstream responses are buffered first, then emitted as SSE so clients still get OpenAI-style stream chunks.

</details>

<details><summary><strong>Out of scope</strong></summary>

### Out of scope

- **Client request rate limiting:** Not implemented; use a reverse proxy if you need it.
- **HTTPS / TLS termination:** Listen plain HTTP; terminate TLS in front (Caddy, etc.).
- **Account pools / OAuth:** Single ChatJimmy upstream endpoint; no session store, token refresh, or multi-account load balancing.
- **Native function calling:** Tool use is emulated in the prompt/response layer, not via native model APIs.
- **Multiple upstream models:** ChatJimmy currently exposes Llama 3.1 8B; the gateway advertises configured model ids but forwards to that upstream service.
- **Model quality expectations:** Llama 3.1 8B on ChatJimmy is aggressively quantized (3-bit/6-bit), so quality is below typical GPU baselines.
- **True upstream streaming:** Time-to-first-token equals full generation time because the gateway buffers ChatJimmy's response before emitting SSE.

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
# or
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

**Request body** for `POST /v1/chat/completions`:

| Field         | Type             | Default       | Description                                                                        |
| ------------- | ---------------- | ------------- | ---------------------------------------------------------------------------------- |
| `model`       | string           | `llama3.1-8B` | Model id                                                                           |
| `messages`    | array            | required      | `{role, content}` messages (`system`, `user`, `assistant`, `tool`)                 |
| `stream`      | boolean          | `false`       | Enable SSE streaming                                                               |
| `tools`       | array            | `[]`          | OpenAI-format tool definitions (filtered, see [Features](#features))               |
| `tool_choice` | string \| object | `"auto"`      | `"auto"`, `"none"`, `"required"`, or `{"type":"function","function":{"name":"…"}}` |

Example requests:

```sh
curl -s http://127.0.0.1:8080/v1/models

curl -s http://127.0.0.1:8080/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -d '{"model":"llama3.1-8B","messages":[{"role":"user","content":"Hello"}]}'
```

When `stream: true`, responses use SSE chunks in the standard `chat.completion.chunk` format.

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

`jimmy-gateway` is configured via a `.env` file or environment variables. Defaults work out of the box—no setup required to proxy ChatJimmy.

```bash
PORT=8080
# API_KEY=your-secret   # optional: require Bearer auth on /v1/chat/completions
```

| Variable  | Default | When to set                                          |
| --------- | ------- | ---------------------------------------------------- |
| `PORT`    | `8080`  | Change listen port (like `proxy.py --port`)          |
| `API_KEY` | (off)   | Lock down the gateway; also accepts `OPENAI_API_KEY` |

ChatJimmy upstream URL and model are built in: `https://chatjimmy.ai/api/chat`, model `llama3.1-8B`. Run `jimmy-gateway -h` for CLI flags.

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
