# CLAUDE.md — Domestic System

## Project Overview
Terminal Ethereum wallet browser built on Charm.sh's Bubble Tea TUI stack.
Go module: `charm-wallet-tui`. No private key handling. No on-chain broadcasting.
The app is purely read + transaction-packaging.

---

## Commands

```bash
# Build
go build ./...

# Run
go run .

# Vet (run after any structural change)
go vet ./...

# Test all
go test ./...

# Test with live RPC (required for rpc/ and some helpers/ tests)
ETH_RPC_URL="https://eth.llamarpc.com" go test ./... -v

# Test specific package
ETH_RPC_URL="https://eth.llamarpc.com" go test ./rpc -v -run TestConnect
```

Tests in `rpc/rpc_test.go` and `helpers/uniswap_test.go` skip without `ETH_RPC_URL`.

---

## File Map

| Path | Role |
|---|---|
| `main.go` | Entry point — wires Bubble Tea program |
| `model.go` | `model` struct + `newModel()` + `Init()` |
| `update.go` | All `Update()` Msg handlers + huh form temp vars |
| `view.go` | Top-level `View()` dispatch |
| `messages.go` | Custom Tea Msg/Cmd definitions |
| `commands.go` | Tea Cmd factory functions (RPC, ENS, etc.) |
| `views/` | Per-page renderers (wallets, details, settings, dapps, uniswap, log) |
| `rpc/rpc.go` | Ethereum RPC client, balance loading, QR generation |
| `rpc/rpc_test.go` | Live integration tests |
| `helpers/helpers.go` | ENS resolution, address formatting, validation |
| `helpers/uniswap.go` | Uniswap V2 swap quote logic |
| `helpers/uniswap_v4_listener.go` | Uniswap V4 event listener (separate from V2 quote logic) |
| `config/config.go` | JSON config load/save, type definitions |
| `styles/styles.go` | All Lip Gloss colors and shared styles |

---

## Architecture

Follows **The Elm Architecture** via Bubble Tea:
- `Init()` → returns initial `tea.Cmd`s (spinner tick, RPC connect)
- `Update(msg)` → pure function returning `(model, tea.Cmd)`
- `View()` → pure function returning a string

**Never bypass this pattern.** Do not mutate model state outside `Update`.

Pages are `config.Page` constants: `PageWallets`, `PageDetails`, `PageSettings`, `PageDapps`, `PageUniswap`.

---

## Code Conventions

### Huh Forms
`huh.Form` fields bind to **package-level vars** in `update.go` (e.g. `tempRPCFormName`, `tempSendToAddr`).
This is intentional — it avoids pointer-to-copy bugs with the model struct. Do not change this pattern.

### Styles
All colors and shared styles live in `styles/styles.go`. Do not define inline `lipgloss.Color` values elsewhere.
The palette is retro-future dark: near-black bg, purple borders, green/blue accents.

| Var | Use |
|---|---|
| `styles.CAccent` | Primary green `#7EE787` |
| `styles.CAccent2` | Blue `#79C0FF` |
| `styles.CBorder` | Purple `#874BFD` |
| `styles.CWarn` | Orange `#FFA657` |
| `styles.CError` | Pink/red `#F25D94` |

### Token Watchlist
Defined in `model.go` inside `newModel()` as `[]rpc.WatchedToken`.
Hardcoded mainnet addresses: WETH, USDC, USDT, DAI. Do not duplicate or move this.

### RPC Timeouts
Connect: 8s. Balance/token loads: 12s. Do not exceed these or remove them.

### Config
Load/save only through `config/config.go`. Config path: `~/.charm-wallet-config.json`.
`ETH_RPC_URL` env var is used as fallback when no RPC URLs are in config.

---

## Hard Constraints

- **No private key storage or signing** anywhere in the codebase.
- **No on-chain transaction broadcasting, except for user-pasted pre-signed transactions**, which may be submitted via `eth_sendRawTransaction` to the user's configured RPC endpoint. Unsigned txs built by the app are still packaged as EIP-681/EIP-4527 QR only — the app never signs anything itself.
- **No telemetry, analytics, or external HTTP** outside user-configured RPC endpoints.
- **No custodial services or hosted backends.**
- Uniswap quoting is V2 pair reserves only (USDC ↔ ETH/WETH). Do not add routing.

---

## Workflow Preferences

- Do not auto-commit changes.
- Run `go vet ./...` after any structural edit.
- Prefer editing existing files over creating new ones.
- Do not add docstrings, comments, or type annotations to code you didn't change.
- Do not add error handling for scenarios internal invariants already prevent.

## Behavioral Rules

**Think before coding.** State assumptions explicitly. Ask rather than guess. Push back when a simpler path exists.
**Plan Mode required for tasks with 3+ steps.** Enter Plan Mode, get approval, then execute. Do not implement and plan simultaneously.
**Use @@file references to anchor context.** When editing a module, reference its file explicitly (e.g., `@@src/signer.ts`) so context stays grounded in actual code, not memory.
**Read before you write.** Before modifying any file, read its exports, immediate callers, and any shared utilities it depends on. Check `src/index.ts` before touching any public API.
**Surgical changes only.** Touch only what the task requires. Do not improve adjacent code, rename variables, or refactor unless explicitly asked.
**Surface conflicts, don't average them.** If two existing patterns contradict, pick the more recent or more tested one, explain the choice, and flag the other for cleanup.
**Checkpoint after every significant step.** State: what was done, what is verified, what remains. Do not continue from a state you cannot describe back.
**Fail loud.** "Done" is wrong if anything was skipped silently. "Tests pass" is wrong if any were skipped or commented out. Surface uncertainty — do not hide it.
**Token budgets are not advisory.** Per-task: 4,000 tokens. Per-session: 30,000 tokens. Approaching the limit? Summarize and start fresh. Surface the breach.
