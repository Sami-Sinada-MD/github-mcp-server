# AGENTS.md

## Cursor Cloud specific instructions

This repo is the **GitHub MCP Server** (Go). It is a headless MCP server (no GUI); it
runs over `stdio` or `http` and exposes GitHub API tools to MCP clients. The other repos
in this workspace (`Fork-For-Realness`, `Medicine-Posture`) have no runnable application.

Standard build/test/lint/run commands live in `CONTRIBUTING.md` and the `script/` directory;
prefer those. Notes below are the non-obvious caveats discovered during environment setup.

### Dependencies
- Environment install runs `go mod download`, `npm ci` in `ui/`, `script/build-ui`, and
  `go build` of `/usr/local/bin/github-mcp-server`. It also installs `golangci-lint`
  v2.9.0 into `./bin` when that binary is missing. You normally do not need to reinstall.

### Building
- `go build ./cmd/github-mcp-server` produces the server binary.
- The Go binary embeds the MCP App UIs via `//go:embed ui_dist/*.html` (`pkg/github/ui_embed.go`).
  A committed `.placeholder.html` lets `go build` succeed **without** building the UI.
  To embed the real UIs, run `script/build-ui` first (builds `ui/` with Vite into
  `pkg/github/ui_dist/*.html`). These outputs are gitignored.

### Testing
- `script/test` runs `go test -race ./...`. Plain `go test ./...` also works and is faster.
- e2e tests (`e2e/`) are gated behind the `e2e` build tag and require Docker **and** a real
  token: `GITHUB_MCP_SERVER_E2E_TOKEN=<token> go test -v --tags e2e ./e2e`.

### Linting
- `script/lint` self-installs `golangci-lint` v2.9.0 into `./bin` (gitignored) and also runs
  `gofmt -s -w .`, which can modify files in place.

### Running
- stdio: `GITHUB_PERSONAL_ACCESS_TOKEN=<token> ./github-mcp-server stdio`. The server
  **exits immediately** if the token env var is unset.
- The token is only checked against GitHub when a tool calls the API. `initialize` and
  `tools/list` succeed with a placeholder. stdio accepts any non-empty
  `GITHUB_PERSONAL_ACCESS_TOKEN`. HTTP mode parses `Authorization: Bearer` and requires
  a GitHub token shape (`ghp_`, `github_pat_`, `gho_`, `ghu_`, `ghs_`, or 40 hex
  characters); `ghp_placeholder` is enough for `initialize` and `tools/list`. A tool
  such as `get_me` needs a real PAT.
- http: `GITHUB_PERSONAL_ACCESS_TOKEN` is not required to start the process. On boot the
  environment runs `/usr/local/bin/github-mcp-server http --port 8082 --listen-host 0.0.0.0`.
  Clients send the token per request as `Authorization: Bearer <token>`. Rebuild and
  `sudo install` over `/usr/local/bin/github-mcp-server` after code changes, then restart
  the listener if you need the new binary (port 8082 is already taken by the boot process).
- Use `--read-only` to restrict to read-only tools; `--toolsets` to select tool groups.
