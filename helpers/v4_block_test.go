package helpers

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"
	"os"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

// ── Constants ─────────────────────────────────────────────────────────────────

const (
	v4TestBlock   = uint64(24686488)
	v4TestAddrHex = "0x5857bCe5490545a89598b9992DD0D409C4C20d86"
	v4TestRPCEnv  = "ETH_RPC_URL"

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
	// ERC-721 Transfer layout: topic[0]=sig, topic[1]=from, topic[2]=to, topic[3]=tokenId.
	// Pinning the address at topic[1] finds all transfers/mints sent FROM the address.
	section(t, fmt.Sprintf("PositionManager topic[1] search · addr=%s · block=%d",
		targetAddr.Hex(), v4TestBlock))

	addrTopic := common.BytesToHash(targetAddr.Bytes())
	t.Logf("  PositionManager : %s", posManager.Hex())
	t.Logf("  Target address  : %s", targetAddr.Hex())
	t.Logf("  topic[1] filter : %s", addrTopic.Hex())

	matchedLogs, filterErr := client.FilterLogs(ctx, ethereum.FilterQuery{
		FromBlock: new(big.Int).SetUint64(v4TestBlock),
		ToBlock:   new(big.Int).SetUint64(v4TestBlock),
		Addresses: []common.Address{posManager},
		Topics:    [][]common.Hash{nil, {addrTopic}},
	})
	if filterErr != nil {
		t.Fatalf("FilterLogs: %v", filterErr)
	}

	if len(matchedLogs) == 0 {
		t.Logf("  RESULT: no PositionManager events with target address at topic[1]")
		return
	}
	t.Logf("  PASS: %d event(s) matched", len(matchedLogs))

	// Deduplicate tx hashes so we only fetch each receipt once.
	seen := map[common.Hash]bool{}
	var uniqueTxHashes []common.Hash
	for _, lg := range matchedLogs {
		if !seen[lg.TxHash] {
			seen[lg.TxHash] = true
			uniqueTxHashes = append(uniqueTxHashes, lg.TxHash)
		}
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
				if ti == 1 && lg.Address == posManager {
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
				if ti == 1 && lg.Address == posManager {
					mark = "  -> "
				}
				t.Logf("  %s[%d] %s", mark, ti, topic.Hex())
			}
			if len(lg.Data) > 0 {
				t.Logf("  data : 0x%s", hex.EncodeToString(lg.Data))
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
