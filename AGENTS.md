# AGENTS.md

## Cursor Cloud specific instructions

This repo is the **GitHub MCP Server** (Go). It is a headless MCP server (no GUI); it
runs over `stdio` or `http` and exposes GitHub API tools to MCP clients. The other repos
in this workspace (`Fork-For-Realness`, `Medicine-Posture`) have no runnable application.

Standard build/test/lint/run commands live in `CONTRIBUTING.md` and the `script/` directory;
prefer those. Notes below are the non-obvious caveats discovered during environment setup.

### Dependencies
- The VM startup update script already runs `go mod download` and installs the UI deps
  (`npm ci` in `ui/`). You normally do not need to reinstall.

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
- The token is only validated when a tool actually calls GitHub, so `initialize` +
  `tools/list` work with any non-empty placeholder token; calling a tool like `get_me`
  requires a real PAT.
- http: `GITHUB_PERSONAL_ACCESS_TOKEN=<token> ./github-mcp-server http --port 8082`.
  In http mode the client sends the token per-request as `Authorization: Bearer <token>`.
- Use `--read-only` to restrict to read-only tools; `--toolsets` to select tool groups.
