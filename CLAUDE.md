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
| `helpers/uniswap.go` | Uniswap V2 swap quote logic + `ResolvePairOnChain` (V2/V3/V4 dispatch) |
| `helpers/uniswap_v3.go` | Uniswap V3 swap quote logic (QuoterV2 simulation) |
| `helpers/uniswap_v4_quote.go` | Uniswap V4 swap quote logic (V4Quoter simulation) + V4 pool resolution |
| `helpers/uniswap_v4_listener.go` | Uniswap V4 event listener + on-demand pool reads (separate from quote logic) |
| `helpers/ondo_tokens.go` | Vendored Ondo Global Markets token list (`go:embed`), all ~440 tokens |
| `helpers/ondo_v4_pools.go` | Vendored Ondo Global Markets V4 pool discovery index (`go:embed`) |
| `helpers/ondo_liquid_tokens.go` | Vendored subset of Ondo tokens with confirmed live liquidity (`go:embed`); seeds the default watchlist |
| `cmd/updateondotokens/` | Dev tool: refreshes `helpers/data/ondo_gm_tokens.json` from Ondo's official GitHub token list |
| `cmd/discoverondopools/` | Dev tool: scans `PoolManager` Initialize events to rebuild `helpers/data/ondo_v4_pools.json` |
| `cmd/discoverondoliquidity/` | Dev tool: re-checks all Ondo tokens for live V2/V3/V4 liquidity, rebuilds `helpers/data/ondo_liquid_tokens.json` |
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
Defined in `model_helpers.go`'s `buildTokenWatchlist()`, seeded into `model.go`'s `newModel()`
only when the loaded config has no `WatchedTokens` yet (fresh/reset configs) — existing users'
saved watchlists are never modified by changing the defaults here.
Hardcoded mainnet addresses: WETH, USDC, USDT, DAI. Do not duplicate or move this.

`buildTokenWatchlist` also seeds `helpers.OndoLiquidTokens` (vendored, `helpers/ondo_liquid_tokens.go`,
rebuilt by `cmd/discoverondoliquidity`) — the subset of Ondo Global Markets tokens confirmed to have
live V2/V3/V4 liquidity as of that tool's last run. This is a snapshot, not a live check; liquidity
shifts, so re-run the tool periodically rather than hand-editing the vendored JSON. Empirically
measured at ~220ms for the full ~27-token default list against a local node — comfortably inside
the 12s balance-load timeout, but re-check if this list grows much further.

The Watched Tokens page's Ondo picker (`o` key, `dialogOndoPicker`) autofills the add-token
form's address from `helpers.OndoGMTokenList` (vendored, `helpers/ondo_tokens.go`, all ~440 Ondo
tokens regardless of liquidity) but always runs the existing on-chain `symbol()`/`decimals()`
verification before adding — the vendored list's Symbol/Decimals are never trusted directly into
`m.tokenWatch`. Do not bulk-add the full picker list to the default watchlist; it's 400+ tokens
and most have no liquidity — that's what `OndoLiquidTokens` filters down to.

### RPC Timeouts
Connect: 8s. Balance/token loads: 12s. Do not exceed these or remove them.

### Config
Load/save only through `config/config.go`. Config path: `~/.charm-wallet-config.json`.
`ETH_RPC_URL` env var is used as fallback when no RPC URLs are in config.

---

## Hard Constraints

- **No private key storage or signing** anywhere in the codebase.
- **No on-chain transaction broadcasting, except for user-pasted pre-signed transactions**, which may be submitted via `eth_sendRawTransaction` to the user's configured RPC endpoint. Unsigned txs built by the app are still packaged as EIP-681/EIP-4527 QR only — the app never signs anything itself.
- **No telemetry, analytics, or external HTTP** outside user-configured RPC endpoints, with one narrow exception: `cmd/updateondotokens` and `cmd/discoverondopools` are maintainer-run dev tools, never invoked by the shipped TUI binary (`go run .` / the built app never calls them) — the same category as `go mod tidy` fetching a module. Anything reachable from the running app still must not make outside HTTP calls.
- **No custodial services or hosted backends.**
- Uniswap quoting resolves pools fully on-chain across V2 (pair reserves), V3 (QuoterV2 simulation), and V4 (V4Quoter simulation) — see `helpers.ResolvePairOnChain`. V4 has no on-chain factory/registry to query live (a pool's existence is only knowable from having observed its `Initialize` event), so V4 resolution uses a vendored discovery index (`helpers/data/ondo_v4_pools.json`, rebuilt by `cmd/discoverondopools`) plus a bounded recent-block fallback scan — never an unbounded scan or a hand-typed pair-address table. Do not add multi-hop routing; every quote/swap is single-pool, single-hop, matching V2/V3 scope.

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
