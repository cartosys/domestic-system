package helpers

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

// ── Constants ─────────────────────────────────────────────────────────────────

const (
	v4TestBlock   = uint64(24686488)
	v4TestAddrHex = "0x5857bCe5490545a89598b9992DD0D409C4C20d86"
	v4TestRPCEnv  = "ETH_RPC_URL"

	// v4TestTxHash is the transaction in which the test address received a
	// Uniswap V4 liquidity position at block 24686488.
	v4TestTxHash = "0x28dd5ac87549b6098c3fc0fa321b926c11ae8adab8aded55f62a5a50873cf7ae"

	v4PositionManagerAddress = "0xbD216513d74C8cf14cf4747E6AaA6420FF64ee9e"
)

// ── Formatting helpers ────────────────────────────────────────────────────────

func hr(t *testing.T) {
	t.Helper()
	t.Logf("─────────────────────────────────────────────────────────────────────────")
}

func section(t *testing.T, title string) {
	t.Helper()
	t.Logf("")
	t.Logf("════════════════════════════════════════════════════════════════════════")
	t.Logf("  %s", title)
	t.Logf("════════════════════════════════════════════════════════════════════════")
}

func addrOrCreate(a *common.Address) string {
	if a == nil {
		return "(contract creation)"
	}
	return a.Hex()
}

// ── Main test ─────────────────────────────────────────────────────────────────
//
// TestV4PositionManagerFrom searches the Uniswap V4 PositionManager for events
// where the target address appears at topic[1] (the ERC-721 "from" field) in
// block 24686488. For every matching tx it prints the full transaction and all
// receipt logs with their topics.
//
// Run with:
//
//	ETH_RPC_URL="https://eth.llamarpc.com" go test ./helpers -v -run TestV4PositionManagerFrom
func TestV4PositionManagerFrom(t *testing.T) {
	rpcURL := os.Getenv(v4TestRPCEnv)
	if rpcURL == "" {
		t.Skipf("%s not set — skipping live V4 block test", v4TestRPCEnv)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client, err := ethclient.DialContext(ctx, rpcURL)
	if err != nil {
		t.Fatalf("dial RPC: %v", err)
	}
	defer client.Close()

	targetAddr := common.HexToAddress(v4TestAddrHex)
	posManager := common.HexToAddress(v4PositionManagerAddress)
	signer := types.LatestSignerForChainID(big.NewInt(1))

	// ── Fetch block (needed for tx lookup) ────────────────────────────────────
	section(t, fmt.Sprintf("Block %d", v4TestBlock))
	block, err := client.BlockByNumber(ctx, new(big.Int).SetUint64(v4TestBlock))
	if err != nil {
		t.Fatalf("fetch block: %v", err)
	}
	t.Logf("  Hash       : %s", block.Hash().Hex())
	t.Logf("  Timestamp  : %d", block.Time())
	t.Logf("  Tx count   : %d", len(block.Transactions()))
	t.Logf("  Gas used   : %d / %d", block.GasUsed(), block.GasLimit())
	t.Logf("  Base fee   : %s wei", block.BaseFee().String())
	t.Logf("  Miner      : %s", block.Coinbase().Hex())

	// ── PositionManager topic[1] filter ───────────────────────────────────────
	// Fetch all PositionManager events in the block, then match the target
	// address by substring search across every topic. Addresses are ABI-encoded
	// as 32-byte left-zero-padded values, so the 40-char hex of the address
	// (without 0x) always appears verbatim inside the topic hex string.
	// This works regardless of which topic position the address occupies
	// (topic[1]=from for transfers, topic[2]=to for mints, etc.).
	section(t, fmt.Sprintf("PositionManager address search · addr=%s · block=%d",
		targetAddr.Hex(), v4TestBlock))

	needle := strings.ToLower(strings.TrimPrefix(targetAddr.Hex(), "0x"))
	t.Logf("  PositionManager : %s", posManager.Hex())
	t.Logf("  Target address  : %s", targetAddr.Hex())
	t.Logf("  Needle          : %s", needle)

	allPosLogs, filterErr := client.FilterLogs(ctx, ethereum.FilterQuery{
		FromBlock: new(big.Int).SetUint64(v4TestBlock),
		ToBlock:   new(big.Int).SetUint64(v4TestBlock),
		Addresses: []common.Address{posManager},
	})
	if filterErr != nil {
		t.Fatalf("FilterLogs: %v", filterErr)
	}
	t.Logf("  Total PositionManager events in block: %d", len(allPosLogs))

	var matchedLogs []types.Log
	for _, lg := range allPosLogs {
		for _, topic := range lg.Topics {
			if strings.Contains(strings.ToLower(topic.Hex()), needle) {
				matchedLogs = append(matchedLogs, lg)
				break
			}
		}
	}

	if len(matchedLogs) == 0 {
		t.Logf("  RESULT: address not found in any PositionManager topic at block %d", v4TestBlock)
		return
	}
	t.Logf("  PASS: %d event(s) contain target address in a topic", len(matchedLogs))

	// Deduplicate tx hashes so we only fetch each receipt once.
	seen := map[common.Hash]bool{}
	var uniqueTxHashes []common.Hash
	for _, lg := range matchedLogs {
		if !seen[lg.TxHash] {
			seen[lg.TxHash] = true
			uniqueTxHashes = append(uniqueTxHashes, lg.TxHash)
		}
	}

	t.Logf("  Matched tx hashes (%d):", len(uniqueTxHashes))
	for i, h := range uniqueTxHashes {
		t.Logf("    [%d] %s", i, h.Hex())
	}

	// ── Per-tx output ─────────────────────────────────────────────────────────
	for txIdx, txHash := range uniqueTxHashes {
		section(t, fmt.Sprintf("Transaction %d / %d  ·  %s", txIdx+1, len(uniqueTxHashes), txHash.Hex()))

		// Find tx in block (avoids eth_getTransactionByHash which some providers
		// return "not found" for on archived blocks).
		var tx *types.Transaction
		for _, btx := range block.Transactions() {
			if btx.Hash() == txHash {
				tx = btx
				break
			}
		}

		if tx == nil {
			t.Logf("  (tx not found in block — skipping)")
			continue
		}

		from, _ := types.Sender(signer, tx)

		// ── Transaction fields ─────────────────────────────────────────────
		t.Logf("  From       : %s", from.Hex())
		t.Logf("  To         : %s", addrOrCreate(tx.To()))
		t.Logf("  Hash       : %s", txHash.Hex())
		t.Logf("  Type       : %d (0=legacy 1=accessList 2=EIP-1559 3=blob)", tx.Type())
		t.Logf("  Nonce      : %d", tx.Nonce())
		t.Logf("  Value      : %s wei", tx.Value().String())
		t.Logf("  Gas limit  : %d", tx.Gas())
		t.Logf("  Gas price  : %s wei", tx.GasPrice().String())
		if tx.Type() >= 2 {
			t.Logf("  Tip cap    : %s wei", tx.GasTipCap().String())
			t.Logf("  Fee cap    : %s wei", tx.GasFeeCap().String())
		}
		t.Logf("  Data size  : %d bytes", len(tx.Data()))
		if len(tx.Data()) >= 4 {
			t.Logf("  Selector   : 0x%s", hex.EncodeToString(tx.Data()[:4]))
		}
		if len(tx.Data()) > 0 {
			t.Logf("  Calldata   : 0x%s", hex.EncodeToString(tx.Data()))
		}

		// ── Receipt ───────────────────────────────────────────────────────
		receipt, rcptErr := client.TransactionReceipt(ctx, txHash)
		if rcptErr != nil {
			t.Logf("  receipt error: %v", rcptErr)
			continue
		}

		statusStr := "FAILED"
		if receipt.Status == 1 {
			statusStr = "SUCCESS"
		}
		t.Logf("  Status     : %s", statusStr)
		t.Logf("  Gas used   : %d (%.1f%% of limit)", receipt.GasUsed,
			float64(receipt.GasUsed)/float64(tx.Gas())*100)
		t.Logf("  Cumul gas  : %d", receipt.CumulativeGasUsed)
		t.Logf("  Tx index   : %d", receipt.TransactionIndex)
		if receipt.ContractAddress != (common.Address{}) {
			t.Logf("  Created    : %s", receipt.ContractAddress.Hex())
		}

		// ── Internal call trace (debug_traceTransaction) ──────────────────
		// Event logs cannot show internal ETH transfers or nested calls.
		// The call tracer exposes the full execution tree.
		t.Logf("")
		t.Logf("  ── Internal call trace ──────────────────────────────────────────")
		var callTrace callFrame
		traceErr := client.Client().Call(&callTrace, "debug_traceTransaction",
			txHash, map[string]string{"tracer": "callTracer"})
		if traceErr != nil {
			t.Logf("  (debug_traceTransaction unavailable: %v)", traceErr)
		} else {
			printCallTree(t, callTrace, 0, needle)
		}

		// ── Flat topic summary ─────────────────────────────────────────────
		totalTopics := 0
		for _, lg := range receipt.Logs {
			totalTopics += len(lg.Topics)
		}
		t.Logf("")
		t.Logf("  ── All topics (%d total across %d logs) ─────────────────────────", totalTopics, len(receipt.Logs))
		n := 0
		for li, lg := range receipt.Logs {
			for ti, topic := range lg.Topics {
				mark := "  "
				if strings.Contains(strings.ToLower(topic.Hex()), needle) {
					mark = "->"
				}
				t.Logf("  %s [log %2d · topic %d · #%3d]  %s  (emitter %s)",
					mark, li, ti, n, topic.Hex(), lg.Address.Hex())
				n++
			}
		}

		// ── Per-log detail ─────────────────────────────────────────────────
		t.Logf("")
		t.Logf("  ── Per-log detail (%d logs) ──────────────────────────────────────", len(receipt.Logs))
		for li, lg := range receipt.Logs {
			t.Logf("")
			t.Logf("  log [%d/%d]  emitter=%s  logIndex=%d  tx=%s",
				li+1, len(receipt.Logs), lg.Address.Hex(), lg.Index, lg.TxHash.Hex())
			t.Logf("  topics (%d):", len(lg.Topics))
			for ti, topic := range lg.Topics {
				mark := "     "
				if strings.Contains(strings.ToLower(topic.Hex()), needle) {
					mark = "  -> "
				}
				t.Logf("  %s[%d] %s", mark, ti, topic.Hex())
			}
			if len(lg.Data) == 0 {
				t.Logf("  data : (empty)")
			} else {
				t.Logf("  data : %d bytes  0x%s", len(lg.Data), hex.EncodeToString(lg.Data))
				for w := 0; w < len(lg.Data); w += 32 {
					end := w + 32
					if end > len(lg.Data) {
						end = len(lg.Data)
					}
					t.Logf("  word [%2d] 0x%s", w/32, hex.EncodeToString(lg.Data[w:end]))
				}
			}
			hr(t)
		}
	}

	section(t, "Done")
}

// ── Decoders (kept for use by v4_block_scanner.go) ───────────────────────────

// decodeERC721Transfer prints an ERC-721 Transfer event.
// topic layout: [sig, from (indexed), to (indexed), tokenId (indexed)]
func decodeERC721Transfer(t *testing.T, lg *types.Log) {
	t.Helper()
	if len(lg.Topics) < 4 {
		t.Logf("    [ERC-721 Transfer] too few topics (%d)", len(lg.Topics))
		return
	}
	from := common.BytesToAddress(lg.Topics[1].Bytes()[12:])
	to := common.BytesToAddress(lg.Topics[2].Bytes()[12:])
	tokenID := new(big.Int).SetBytes(lg.Topics[3].Bytes())
	mintOrTransfer := "Transfer"
	if (from == common.Address{}) {
		mintOrTransfer = "Mint"
	}
	t.Logf("    [ERC-721 %s]  contract=%s  from=%s  to=%s  tokenId=%s  tx=%s",
		mintOrTransfer,
		lg.Address.Hex(),
		from.Hex(), to.Hex(),
		tokenID.String(),
		lg.TxHash.Hex(),
	)
}

// decodeIncreaseLiquidity prints an IncreaseLiquidity event.
// topic layout: [sig, tokenId (indexed)]
// data: liquidity uint128, amount0 uint128, amount1 uint128
func decodeIncreaseLiquidity(t *testing.T, lg *types.Log) {
	t.Helper()
	if len(lg.Topics) < 2 {
		t.Logf("    [IncreaseLiquidity] too few topics (%d)", len(lg.Topics))
		return
	}
	tokenID := new(big.Int).SetBytes(lg.Topics[1].Bytes())
	if len(lg.Data) < 96 {
		t.Logf("    [IncreaseLiquidity]  tokenId=%s  (data too short: %d bytes)", tokenID.String(), len(lg.Data))
		return
	}
	liquidity := new(big.Int).SetBytes(lg.Data[0:32])
	amount0 := new(big.Int).SetBytes(lg.Data[32:64])
	amount1 := new(big.Int).SetBytes(lg.Data[64:96])
	t.Logf("    [IncreaseLiquidity]  contract=%s  tokenId=%s  liquidity=%s  amount0=%s  amount1=%s  tx=%s",
		lg.Address.Hex(),
		tokenID.String(),
		liquidity.String(), amount0.String(), amount1.String(),
		lg.TxHash.Hex(),
	)
}

// appendPoolID appends id to ids only if not already present.
func appendPoolID(ids []common.Hash, id common.Hash) []common.Hash {
	for _, existing := range ids {
		if existing == id {
			return ids
		}
	}
	return append(ids, id)
}

// ── Pool-creation chain test ──────────────────────────────────────────────────
//
// TestV4PoolCreationFromPositionManager walks the full V4 data chain starting
// from a known transaction in which the test address received a liquidity position.
//
// V4 ownership design — why we start from the tx receipt, not from event topics:
//
//   In Uniswap V4 the PositionManager batches operations via modifyLiquidities().
//   The primary events it emits are:
//     • IncreaseLiquidity(uint256 indexed tokenId, ...) — tokenId only, no address
//     • ERC-721 Transfer(from, to, tokenId) — owner IS here, but only on fresh mints
//   For INCREASE_LIQUIDITY actions on an existing position no Transfer is emitted
//   at all.  The recipient address is therefore not reliably present in any
//   PositionManager topic.
//
//   The authoritative source for "who holds a position" is the ERC-721 on-chain
//   state (ownerOf / balanceOf / tokenOfOwnerByIndex), not event scanning.
//   For historical indexing, ERC-721 Transfer events track ownership changes, but
//   IncreaseLiquidity events do not.
//
//   For pool creation: Initialize(poolId, currency0, currency1, fee, tickSpacing,
//   hooks, sqrtPrice, tick) has no creator field — creator is only in tx.from.
//
// Chain walked by this test:
//  1. Fetch the tx receipt for the known tx hash.
//  2. Scan every log in the receipt for IncreaseLiquidity → extract tokenId(s).
//  3. Call positions(tokenId) on the PositionManager → pool key.
//  4. Compute poolId = keccak256(abi.encode(poolKey)).
//  5. Query PoolManager for the Initialize event (pool creation) via poolId topic.
//  6. Query PoolManager for all Swap events for that pool from creation → test block.
//  7. Query PoolManager for all ModifyLiquidity events for that pool.
//
// Run with:
//
//	ETH_RPC_URL="https://eth.llamarpc.com" go test ./helpers -v -run TestV4PoolCreationFromPositionManager
func TestV4PoolCreationFromPositionManager(t *testing.T) {
	rpcURL := os.Getenv(v4TestRPCEnv)
	if rpcURL == "" {
		t.Skipf("%s not set — skipping live V4 pool-creation test", v4TestRPCEnv)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	client, err := ethclient.DialContext(ctx, rpcURL)
	if err != nil {
		t.Fatalf("dial RPC: %v", err)
	}
	defer client.Close()

	targetAddr := common.HexToAddress(v4TestAddrHex)
	poolManager := common.HexToAddress(V4PoolManagerAddress)
	blockBig    := new(big.Int).SetUint64(v4TestBlock)
	needle      := strings.ToLower(strings.TrimPrefix(targetAddr.Hex(), "0x"))

	increaseLiqSig := common.HexToHash("0x3067048beee31b25b2f1681f88dac838c8bba36af25bfb2b7cf7473a5847e35f")
	initializeSig  := crypto.Keccak256Hash([]byte("Initialize(bytes32,address,address,uint24,int24,address,uint160,int24)"))
	swapSig        := crypto.Keccak256Hash([]byte("Swap(bytes32,address,int128,int128,uint160,uint128,int24,uint24)"))
	modifyLiqSig   := crypto.Keccak256Hash([]byte("ModifyLiquidity(bytes32,address,int24,int24,int256,bytes32)"))

	const v4DeployBlock = uint64(21_688_000)

	// ── Step 1: Fetch the tx receipt and show all logs ────────────────────────
	// We start from the known tx hash rather than trying to find the address in
	// event topics.  The receipt contains every log emitted by every contract
	// called in the transaction, giving a complete picture regardless of which
	// contract emits what.
	section(t, fmt.Sprintf("Step 1  Tx receipt  %s", v4TestTxHash))

	txHash := common.HexToHash(v4TestTxHash)
	var receipt *types.Receipt
	for attempt := 0; attempt < 5; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt*2) * time.Second)
		}
		var rcptErr error
		receipt, rcptErr = client.TransactionReceipt(ctx, txHash)
		if rcptErr == nil {
			break
		}
		msg := strings.ToLower(rcptErr.Error())
		if !strings.Contains(msg, "429") && !strings.Contains(msg, "too many") {
			t.Fatalf("TransactionReceipt: %v", rcptErr)
		}
		t.Logf("  (rate limited on receipt fetch, retrying %d/5)", attempt+1)
	}
	if receipt == nil {
		t.Fatalf("TransactionReceipt: failed after retries (rate limited)")
	}

	status := "FAILED"
	if receipt.Status == 1 {
		status = "SUCCESS"
	}
	t.Logf("  block=%d  status=%s  logs=%d  gasUsed=%d",
		receipt.BlockNumber.Uint64(), status, len(receipt.Logs), receipt.GasUsed)

	for i, lg := range receipt.Logs {
		sigStr := "(no topics)"
		if len(lg.Topics) > 0 {
			sigStr = lg.Topics[0].Hex()[:18] + "…"
		}
		mark := "  "
		for _, topic := range lg.Topics {
			if strings.Contains(strings.ToLower(topic.Hex()), needle) {
				mark = "★ "
				break
			}
		}
		t.Logf("  %s[log %2d]  emitter=%s  sig=%s  topics=%d  data=%d bytes",
			mark, i, lg.Address.Hex(), sigStr, len(lg.Topics), len(lg.Data))
	}

	// ── Step 2: Extract tokenId(s) from PositionManager events ──────────────
	// Two event types can carry a tokenId depending on the action:
	//
	//   MINT_POSITION   → ERC-721 Transfer(from=0x0, to=owner, tokenId indexed at topic[3])
	//                     Emitted only on fresh mints; no IncreaseLiquidity follows.
	//
	//   INCREASE_LIQUIDITY → IncreaseLiquidity(uint256 indexed tokenId, ...)  topic[1]=tokenId
	//                        Emitted on adds to an existing position; no Transfer follows.
	//
	// We scan for both so the test covers either transaction type.
	section(t, "Step 2  PositionManager events → tokenIds")

	posManagerAddr := common.HexToAddress(v4PositionManagerAddress)
	// ERC-721 Transfer sig = keccak256("Transfer(address,address,uint256)")
	erc721TransferSig := crypto.Keccak256Hash([]byte("Transfer(address,address,uint256)"))
	zeroAddr := common.Hash{} // from=0x0 for mints

	seenIDs := map[string]bool{}
	var tokenIds []*big.Int
	for _, lg := range receipt.Logs {
		if lg.Address != posManagerAddr {
			continue
		}
		if len(lg.Topics) < 2 {
			continue
		}
		switch {
		case lg.Topics[0] == erc721TransferSig && len(lg.Topics) == 4 && lg.Topics[1] == zeroAddr:
			// Fresh mint: Transfer(from=0x0, to, tokenId)
			tokenId := new(big.Int).SetBytes(lg.Topics[3].Bytes())
			to := common.BytesToAddress(lg.Topics[2].Bytes()[12:])
			t.Logf("  [ERC-721 Mint]  to=%s  tokenId=%s", to.Hex(), tokenId.String())
			if key := tokenId.String(); !seenIDs[key] {
				seenIDs[key] = true
				tokenIds = append(tokenIds, tokenId)
			}

		case lg.Topics[0] == increaseLiqSig:
			// Liquidity increase on existing position
			tokenId := new(big.Int).SetBytes(lg.Topics[1].Bytes())
			decodeIncreaseLiquidity(t, lg)
			if key := tokenId.String(); !seenIDs[key] {
				seenIDs[key] = true
				tokenIds = append(tokenIds, tokenId)
			}
		}
	}

	if len(tokenIds) == 0 {
		t.Fatalf("no PositionManager mint or IncreaseLiquidity events found in tx %s — cannot derive tokenId", v4TestTxHash)
	}
	t.Logf("  tokenIds found: %v", tokenIds)

	// ── Step 3: Extract poolId(s) from the receipt ───────────────────────────
	// Strategy A (preferred, no archive node required):
	//   The PoolManager emits ModifyLiquidity or Swap events with the poolId
	//   at topic[1].  The same tx that mints a position also calls
	//   ModifyLiquidity on the pool, so the poolId is already in the receipt.
	//
	// Strategy B (fallback, requires archive node):
	//   Call positions(tokenId) on the PositionManager at the mint block to
	//   retrieve the full pool key, then compute poolId from it.
	section(t, "Step 3  Receipt PoolManager events → poolId(s)")

	var poolIds []common.Hash

	// Strategy A: scan all PoolManager logs in the receipt.
	t.Logf("  Strategy A: scan receipt for PoolManager ModifyLiquidity / Swap topic[1]=poolId")
	for _, lg := range receipt.Logs {
		if lg.Address != poolManager {
			continue
		}
		if len(lg.Topics) < 2 {
			continue
		}
		sig := lg.Topics[0]
		if sig != modifyLiqSig && sig != swapSig && sig != initializeSig {
			continue
		}
		poolId := lg.Topics[1]
		t.Logf("  found poolId=%s  (event sig=%s…  log=%d)",
			poolId.Hex(), sig.Hex()[:18], lg.Index)
		poolIds = appendPoolID(poolIds, poolId)
	}

	// Strategy B: fallback via positions(tokenId) if Strategy A found nothing.
	if len(poolIds) == 0 {
		t.Logf("  Strategy A found nothing — falling back to positions(tokenId) at block %d", v4TestBlock)
		for _, tokenId := range tokenIds {
			pos, posErr := v4FetchPosition(ctx, client, tokenId, blockBig)
			if posErr != nil {
				t.Logf("  positions(%s) error: %v", tokenId, posErr)
				continue
			}
			t.Logf("  tokenId=%s  currency0=%s  currency1=%s  fee=%d  tickSpacing=%d  hooks=%s",
				tokenId, pos.Token0.Hex(), pos.Token1.Hex(), pos.Fee, pos.TickSpacing, pos.Hooks.Hex())
			poolId := v4ComputePoolId(pos.Token0, pos.Token1, pos.Hooks, pos.Fee, pos.TickSpacing)
			t.Logf("    poolId = %s", poolId.Hex())
			poolIds = appendPoolID(poolIds, poolId)
		}
	}

	if len(poolIds) == 0 {
		t.Fatalf("could not derive any pool IDs from receipt or positions() call — cannot continue")
	}
	t.Logf("  poolIds: %v", poolIds)

	// ── Step 4: PoolManager Initialize event for each poolId ─────────────────
	// poolId is indexed as topic[1] in the Initialize event, so this is a
	// single-round-trip eth_getLogs call over the full deployment range.
	// A pool can only be initialized once, so at most one event is returned.
	section(t, "Step 4  PoolManager Initialize (pool creation)")

	type poolInfo struct {
		PoolId      common.Hash
		CreatedAt   uint64
		InitTxHash  common.Hash
	}
	var createdPools []poolInfo

	// filterLogsChunked splits a block range into ≤1000-block chunks to stay
	// within llamarpc's eth_getLogs limit. Between chunks it sleeps 300ms to
	// avoid rate limits. On 429 or "too many" it backs off 2s and retries (up
	// to 3 times). stopOnFirst=true causes the scan to stop after the first
	// non-empty result (useful for one-time events like Initialize).
	filterLogsChunked := func(q ethereum.FilterQuery, stopOnFirst bool) ([]types.Log, error) {
		const chunkSize = uint64(1000)
		from := q.FromBlock.Uint64()
		to := q.ToBlock.Uint64()
		var all []types.Log
		for start := from; start <= to; start += chunkSize {
			end := start + chunkSize - 1
			if end > to {
				end = to
			}
			chunk := q
			chunk.FromBlock = new(big.Int).SetUint64(start)
			chunk.ToBlock = new(big.Int).SetUint64(end)
			var logs []types.Log
			var err error
			for attempt := 0; attempt < 3; attempt++ {
				if attempt > 0 {
					time.Sleep(2 * time.Second)
				}
				logs, err = client.FilterLogs(ctx, chunk)
				if err == nil {
					break
				}
				msg := strings.ToLower(err.Error())
				if !strings.Contains(msg, "429") && !strings.Contains(msg, "too many") && !strings.Contains(msg, "timeout") {
					return all, err
				}
				t.Logf("  (transient error on chunk %d–%d, retrying %d/3: %v)", start, end, attempt+1, err)
			}
			if err != nil {
				return all, err
			}
			all = append(all, logs...)
			if stopOnFirst && len(all) > 0 {
				break
			}
			time.Sleep(300 * time.Millisecond) // stay under rate limit
		}
		return all, nil
	}

	// maxScanBlocks caps how far back we search for Initialize/Swap/ModifyLiquidity.
	// llamarpc limits eth_getLogs to 1000 blocks per call and ~3 req/s; a window
	// of 10k blocks = 10 API calls which completes in ~5s.  For full historical
	// coverage a dedicated archive node or provider with higher limits is needed.
	const maxScanBlocks = uint64(10_000)
	scanFrom := func(earliest uint64) uint64 {
		if v4TestBlock <= earliest+maxScanBlocks {
			return earliest
		}
		return v4TestBlock - maxScanBlocks
	}

	for _, poolId := range poolIds {
		t.Logf("  poolId=%s", poolId.Hex())
		from4 := scanFrom(v4DeployBlock)
		t.Logf("  Scanning blocks %d–%d for Initialize event (1000-block chunks)…", from4, v4TestBlock)
		initLogs, err := filterLogsChunked(ethereum.FilterQuery{
			FromBlock: new(big.Int).SetUint64(from4),
			ToBlock:   blockBig,
			Addresses: []common.Address{poolManager},
			Topics:    [][]common.Hash{{initializeSig}, {poolId}},
		}, true)
		if err != nil {
			t.Logf("  FilterLogs Initialize: %v", err)
			continue
		}

		if len(initLogs) == 0 {
			t.Logf("  RESULT: no Initialize event found for this pool")
			t.Logf("  (pool may have been created before block %d or on a different network)", v4DeployBlock)
			continue
		}

		lg := initLogs[0] // exactly one: a pool can only be initialized once
		t.Logf("  POOL CREATED")
		t.Logf("    block       = %d", lg.BlockNumber)
		t.Logf("    tx          = %s", lg.TxHash.Hex())
		t.Logf("    logIndex    = %d", lg.Index)
		if len(lg.Topics) >= 4 {
			c0 := common.BytesToAddress(lg.Topics[2].Bytes()[12:])
			c1 := common.BytesToAddress(lg.Topics[3].Bytes()[12:])
			t.Logf("    currency0   = %s", c0.Hex())
			t.Logf("    currency1   = %s", c1.Hex())
		}
		// Non-indexed data layout (5 × 32-byte ABI slots):
		//   [0:32]   fee          uint24
		//   [32:64]  tickSpacing  int24
		//   [64:96]  hooks        address
		//   [96:128] sqrtPriceX96 uint160
		//   [128:160] tick        int24
		if len(lg.Data) >= 160 {
			fee := new(big.Int).SetBytes(lg.Data[0:32]).Uint64()
			tickSpacing := v4DecodeInt24Slot(lg.Data[32:64])
			hooks := common.BytesToAddress(lg.Data[64:96])
			sqrtPrice := new(big.Int).SetBytes(lg.Data[96:128])
			tick := v4DecodeInt24Slot(lg.Data[128:160])
			t.Logf("    fee         = %d  (%.4f%%)", fee, float64(fee)/10000)
			t.Logf("    tickSpacing = %d", tickSpacing)
			t.Logf("    hooks       = %s", hooks.Hex())
			t.Logf("    sqrtPrice   = %s", sqrtPrice.String())
			t.Logf("    initTick    = %d", tick)
		}
		t.Logf("  NOTE: pool creator = tx.from of tx %s", lg.TxHash.Hex())
		t.Logf("        (requires eth_getTransactionByHash — not fetched here)")

		createdPools = append(createdPools, poolInfo{poolId, lg.BlockNumber, lg.TxHash})
	}

	// If Initialize lookup failed (e.g. rate limiting), synthesize poolInfo
	// entries from the poolIds we already have, using v4DeployBlock as the
	// earliest possible start.  Steps 5-6 still produce valid results.
	if len(createdPools) == 0 {
		t.Logf("  Initialize not found — using v4DeployBlock as scan start for Swap/ModifyLiquidity")
		for _, poolId := range poolIds {
			createdPools = append(createdPools, poolInfo{poolId, v4DeployBlock, common.Hash{}})
		}
	}

	// ── Step 5: Swap events for each pool (creation → test block) ────────────
	// Swap.id is indexed as topic[1], so we filter by poolId directly.
	// Swap.sender (topic[2]) is the direct PoolManager caller, typically a
	// router contract — NOT the user's EOA.
	section(t, "Step 5  PoolManager Swap events (creation block → test block)")

	for _, pool := range createdPools {
		swapLogs, swapErr := filterLogsChunked(ethereum.FilterQuery{
			FromBlock: new(big.Int).SetUint64(scanFrom(pool.CreatedAt)),
			ToBlock:   blockBig,
			Addresses: []common.Address{poolManager},
			Topics:    [][]common.Hash{{swapSig}, {pool.PoolId}},
		}, false)
		if swapErr != nil {
			t.Logf("  FilterLogs Swap: %v", swapErr)
			continue
		}
		t.Logf("  pool=%s  Swap events: %d  (blocks %d–%d)",
			pool.PoolId.Hex()[:18]+"…", len(swapLogs), pool.CreatedAt, v4TestBlock)
		for i, lg := range swapLogs {
			if i >= 5 {
				t.Logf("    … %d more swap(s)", len(swapLogs)-5)
				break
			}
			sender := common.Address{}
			if len(lg.Topics) >= 3 {
				sender = common.BytesToAddress(lg.Topics[2].Bytes()[12:])
			}
			t.Logf("    [%d] block=%d  sender=%s  tx=%s", i+1, lg.BlockNumber, sender.Hex(), lg.TxHash.Hex())
		}
	}

	// ── Step 6: ModifyLiquidity events (add/remove liquidity) ─────────────────
	// ModifyLiquidity.id and ModifyLiquidity.sender are both indexed.
	// liquidityDelta > 0 = add liquidity, < 0 = remove liquidity.
	section(t, "Step 6  PoolManager ModifyLiquidity events (creation block → test block)")

	for _, pool := range createdPools {
		modLogs, modErr := filterLogsChunked(ethereum.FilterQuery{
			FromBlock: new(big.Int).SetUint64(scanFrom(pool.CreatedAt)),
			ToBlock:   blockBig,
			Addresses: []common.Address{poolManager},
			Topics:    [][]common.Hash{{modifyLiqSig}, {pool.PoolId}},
		}, false)
		if modErr != nil {
			t.Logf("  FilterLogs ModifyLiquidity: %v", modErr)
			continue
		}
		t.Logf("  pool=%s  ModifyLiquidity events: %d  (blocks %d–%d)",
			pool.PoolId.Hex()[:18]+"…", len(modLogs), pool.CreatedAt, v4TestBlock)
		for i, lg := range modLogs {
			if i >= 5 {
				t.Logf("    … %d more modify(s)", len(modLogs)-5)
				break
			}
			sender := common.Address{}
			if len(lg.Topics) >= 3 {
				sender = common.BytesToAddress(lg.Topics[2].Bytes()[12:])
			}
			// data: tickLower(int24), tickUpper(int24), liquidityDelta(int256), salt(bytes32)
			action := "modify"
			if len(lg.Data) >= 96 {
				// int256 sign: top bit of first byte
				if lg.Data[64]&0x80 != 0 {
					action = "remove"
				} else {
					delta := new(big.Int).SetBytes(lg.Data[64:96])
					if delta.Sign() > 0 {
						action = "add"
					}
				}
			}
			t.Logf("    [%d] block=%d  action=%s  sender=%s  tx=%s",
				i+1, lg.BlockNumber, action, sender.Hex(), lg.TxHash.Hex())
		}
	}

	section(t, "Done")
}

// ── Call trace types and printer ──────────────────────────────────────────────

// callFrame mirrors the output of geth's "callTracer".
type callFrame struct {
	Type    string      `json:"type"`
	From    string      `json:"from"`
	To      string      `json:"to"`
	Value   string      `json:"value,omitempty"`
	Gas     string      `json:"gas"`
	GasUsed string      `json:"gasUsed"`
	Input   string      `json:"input"`
	Output  string      `json:"output,omitempty"`
	Error   string      `json:"error,omitempty"`
	Calls   []callFrame `json:"calls,omitempty"`
}

// printCallTree recursively prints a call frame and all its subcalls.
// needle is the lowercase address (without 0x) used to mark matching lines.
func printCallTree(t *testing.T, f callFrame, depth int, needle string) {
	t.Helper()
	indent := strings.Repeat("    ", depth)

	valueStr := f.Value
	if valueStr == "" || valueStr == "0x0" {
		valueStr = "0"
	}

	fromLower := strings.ToLower(f.From)
	toLower := strings.ToLower(f.To)
	marker := ""
	if strings.Contains(fromLower, needle) || strings.Contains(toLower, needle) {
		marker = "  ★"
	}

	inputSel := ""
	if len(f.Input) >= 10 {
		inputSel = "  sel=" + f.Input[:10]
	}

	errStr := ""
	if f.Error != "" {
		errStr = "  ERR=" + f.Error
	}

	t.Logf("  %s%s  from=%s  to=%s  value=%s  gas=%s  gasUsed=%s%s%s%s",
		indent, f.Type, f.From, f.To, valueStr, f.Gas, f.GasUsed,
		inputSel, errStr, marker)

	for _, sub := range f.Calls {
		printCallTree(t, sub, depth+1, needle)
	}
}
