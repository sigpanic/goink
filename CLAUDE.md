# CLAUDE.md

Goink — desktop AI novel-writing assistant built with Wails (Go + React).

## Build / Dev

```bash
make deps      # download runtime deps (ONNX lib, git, models)
make build     # production build (wails build)
make dev       # dev mode with hot reload (wails dev)
```

**Working directory discipline:**

- **Git commands** — always run from project root (`/home/nianhe/projects/todo`). Before any git operation, ensure CWD is the root.
- **Frontend commands** — always `cd frontend/` first. Build with `npm run build`, never raw `npx vite build` or `npm build`. Install deps with `npm install <pkg> --save` from within `frontend/`. NEVER run `npm install` from project root.
- **Go commands** — run from project root. Use `go build ./...` or `go test ./...`.
- **Know where you are** — before running any command, confirm which directory it will execute in and what side effects it has.

System deps (Ubuntu/Debian): `libsqlite3-dev libgtk-3-dev libwebkit2gtk-4.1-dev gcc`

ONNX Runtime and models are bundled at build time via `scripts/download-onnx.sh` into `build/runtime/`. In dev mode, the ONNX lib fallback path is `~/Goink/runtime/` and models fall back to `~/Goink/models/`. Build constraint `//go:build cgo` guards all ONNX and sqlite-vec code.

## Architecture

```
app/              Wails binding layer — exported methods = frontend API
internal/
  agent/          LLM conversation loop, compression, sub-agents
  agentcfg/       System prompts (system1.go) + context snapshots (system2.go)
  mcp_tools/      30+ MCP tools the AI can call (read, edit, CRUD for all entities)
  llm/            Multi-provider LLM transport (OpenAI-compatible)
  session/        Sessions + messages (append-only, versioned for compression)
  storage/        SQLite init + operation log + rollback
  character/      Character CRUD + directed relationship graph
  timeline/       Foreshadowing entries + 3-slot chapter plans (next/near/far)
  storyarc/       Multi-node story arcs across chapters
  reader/         Reader perspective tracking (known/suspense/misconception)
  location/       Locations as graph: containment tree + undirected spatial edges
  novel/          Novel metadata + global/per-novel preferences
  chapter/        Chapter metadata (content stored as files in git repos)
  git/            File I/O + git version control per novel
  rag/            Vector search (sqlite-vec + ONNX bge-small-zh-v1.5 int8)
  approval/       Blocking approval workflow (manual / auto modes)
  config/         App config, settings, model directory resolution
  platform/       OS-specific paths (AppDir, DataDir, ONNX lib resolution)
  migrate/        Auto-migration
  logger/         Structured slog logging
frontend/         React 19 + TypeScript + Tailwind 4 + shadcn/ui
```

## Key conventions

- **Build tags**: All ONNX and sqlite-vec code uses `//go:build cgo` (see `internal/rag/`, `internal/mcp_tools/memory_tools.go`)
- **Data dir**: `~/Goink/` on Linux/macOS, exe-adjacent on Windows. Contains `models/`, `runtime/`, `novel-agent.db`, `novels/`
- **Per-novel git repos**: Each novel at `{DataDir}/novels/{id}/` with `chapters/NNN.md`, `outlines/NNN.md`, `goink.md`
- **Preferences**: Global + per-novel, free-text category, LLM-classified
- **Character relationships**: Append-only with `is_current` flag — updates create new rows, old rows become history
- **Timeline entries**: `target_chapter` used for ORDER BY only, never WHERE (LLM estimates are imprecise)
- **Messages**: Append-only, versioned for compression. Three query paths: to_api / to_frontend / full audit
- **Commit style**: English, specific, no Co-Authored-By, no emoji
- **Pre-commit hook**: `.githooks/pre-commit` auto-runs validation split by staged file scope — Go changes trigger `go build`/`go test`/`golangci-lint`, frontend changes trigger `npm run build`/`lint`/`test`, docs/config-only commits skip. No manual validation needed before commit.
- **User communicates in Chinese** — respond in Chinese
- **Edit tool + Chinese text**: When Edit tool fails on Go files with Chinese characters ("String to replace not found"), stretch `old_string` to include surrounding ASCII lines as anchors. Before editing, `gofmt -w file.go` and verify indentation with `cat -A`. See `docs/experience/edit-tool-unicode-match-failure.md`.
- **Edit tool + same file**: When editing the same file multiple times, run Edits sequentially, never in parallel. Parallel Edits on the same file race — the later Edit writes based on the pre-edit version, silently overwriting the earlier Edit's change (observed: two Edits on rw_tools.go, the second reverted the first).

## CGO / ONNX notes

- ONNX embedder is a global singleton (`InitEmbedder` / `GetEmbedder`)
- VectorStore is a global singleton (`InitVectorStore` / `GetVectorStore`)
- RefreshQueue is a global singleton with `sync.Once` (`InitRefreshQueue` / `GetRefreshQueue`)
- `ResolveOnnxLib()` search chain: `<appdir>/runtime/` → `~/Goink/runtime/` → system paths
- Models resolution: `<appdir>/runtime/models/` → `~/Goink/models/`
- Vec0 table per novel: `vec_novel_{id}`, cosine distance metric
- Chunks: 420 tokens, 50 overlap, BERT WordPiece tokenizer
- Embedding: 512-dim, CLS pooling + L2 normalization, BGE instruction prefix for queries only

## No-gos

- Never delete or modify logger statements or code comments without explicit request
- Never modify code without user permission — ask first
- Use Edit/Write tools only for code changes, never sed or python scripts
- Don't proactively ask "commit?" or "start writing?" — wait for explicit instruction
- Never hand-edit `frontend/src/lib/wailsjs/go/models.ts` — it is auto-generated by Wails bindings. Always regenerate via `wails generate module` or `make build` instead
- Windows: `platform` build tag issues are expected — cgo code doesn't compile for Windows in diagnostics
