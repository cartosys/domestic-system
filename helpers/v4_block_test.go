package helpers

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

// ── Constants ─────────────────────────────────────────────────────────────────

const (
	v4TestBlock   = uint64(24686488)
	v4TestAddrHex = "0x5857bCe5490545a89598b9992DD0D409C4C20d86"
	v4TestRPCEnv  = "ETH_RPC_URL"

	// V4 PositionManager (NonfungiblePositionManager equivalent for V4) on mainnet.
	// This contract emits ERC-721 Transfer events when LP NFTs are minted.
	v4PositionManagerAddress = "0xbD216513d74C8cf14cf4747E6AaA6420FF64ee9e"
)

// ERC-721 Transfer(address indexed from, address indexed to, uint256 indexed tokenId)
// topic0 = keccak256("Transfer(address,address,uint256)")
const erc721TransferSig = "0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"

// IncreaseLiquidity(uint256 indexed tokenId, uint128 liquidity, uint128 amount0, uint128 amount1)
// topic0 = keccak256("IncreaseLiquidity(uint256,uint128,uint128,uint128)")
const increaseLiqSig = "0x3067048beee31b25b2f1681f88dac838c8bba36af25bfb2b7cf7473a5847e35f"

// ── Helpers ───────────────────────────────────────────────────────────────────

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

func logTxDetails(t *testing.T, tx *types.Transaction, receipt *types.Receipt, idx int) {
	t.Helper()
	section(t, fmt.Sprintf("Transaction #%d", idx+1))

	// Basic tx fields
	t.Logf("  Hash       : %s", tx.Hash().Hex())
	t.Logf("  Type       : %d (0=legacy, 1=access list, 2=EIP-1559, 3=blob)", tx.Type())
	t.Logf("  Nonce      : %d", tx.Nonce())
	t.Logf("  To         : %s", addrOrCreate(tx.To()))
	t.Logf("  Value      : %s wei", tx.Value().String())
	t.Logf("  Gas limit  : %d", tx.Gas())
	t.Logf("  Gas price  : %s wei", tx.GasPrice().String())
	if tx.Type() >= 2 {
		t.Logf("  GasTipCap  : %s wei", tx.GasTipCap().String())
		t.Logf("  GasFeeCap  : %s wei", tx.GasFeeCap().String())
	}
	t.Logf("  Data size  : %d bytes", len(tx.Data()))
	if len(tx.Data()) > 0 {
		t.Logf("  Calldata   : 0x%s", hex.EncodeToString(tx.Data()))
		if len(tx.Data()) >= 4 {
			t.Logf("  Selector   : 0x%s", hex.EncodeToString(tx.Data()[:4]))
		}
	}

	// Receipt fields
	if receipt != nil {
		status := "FAILED"
		if receipt.Status == 1 {
			status = "SUCCESS"
		}
		t.Logf("  Status     : %s", status)
		t.Logf("  GasUsed    : %d (%.1f%% of limit)", receipt.GasUsed,
			float64(receipt.GasUsed)/float64(tx.Gas())*100)
		t.Logf("  CumulGasUsed: %d", receipt.CumulativeGasUsed)
		t.Logf("  Log count  : %d", len(receipt.Logs))
		if receipt.ContractAddress != (common.Address{}) {
			t.Logf("  Contract   : %s (created)", receipt.ContractAddress.Hex())
		}
		t.Logf("  Block      : %d", receipt.BlockNumber.Uint64())
		t.Logf("  TxIndex    : %d", receipt.TransactionIndex)
	}
}

func addrOrCreate(a *common.Address) string {
	if a == nil {
		return "(contract creation)"
	}
	return a.Hex()
}

// ── Main test ─────────────────────────────────────────────────────────────────

// TestV4PoolCreatedAndMint scans block 24686488 for all transactions sent by
// 0x5857bCe5490545a89598b9992DD0D409C4C20d86, then decodes and prints every
// event emitted — with special handling for V4 Initialize (pool creation),
// V4 ModifyLiquidity (LP provision), and ERC-721 Transfer (NFT LP mint).
//
// Run with:
//
//	ETH_RPC_URL="https://eth.llamarpc.com" go test ./helpers -v -run TestV4PoolCreatedAndMint
func TestV4PoolCreatedAndMint(t *testing.T) {
	rpcURL := os.Getenv(v4TestRPCEnv)
	if rpcURL == "" {
		t.Skipf("%s not set — skipping live V4 block test", v4TestRPCEnv)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	client, err := ethclient.DialContext(ctx, rpcURL)
	if err != nil {
		t.Fatalf("dial RPC: %v", err)
	}
	defer client.Close()

	targetAddr := common.HexToAddress(v4TestAddrHex)
	poolManager := common.HexToAddress(V4PoolManagerAddress)

	// ── Parse V4 ABI ──────────────────────────────────────────────────────────
	parsedABI, err := abi.JSON(strings.NewReader(poolManagerEventsABI))
	if err != nil {
		t.Fatalf("parse V4 ABI: %v", err)
	}
	eventNames := make(map[common.Hash]string, len(parsedABI.Events))
	for name, ev := range parsedABI.Events {
		eventNames[ev.ID] = name
	}

	var (
		mu       sync.RWMutex
		poolKeys = make(map[common.Hash]v4PoolKey)
		syms     = newV4SymbolCache()
	)

	// ── Step 1: Fetch full block ───────────────────────────────────────────────
	section(t, fmt.Sprintf("Block %d  ·  Scanning txs from %s", v4TestBlock, targetAddr.Hex()))
	block, err := client.BlockByNumber(ctx, new(big.Int).SetUint64(v4TestBlock))
	if err != nil {
		t.Fatalf("fetch block: %v", err)
	}
	t.Logf("  Block hash     : %s", block.Hash().Hex())
	t.Logf("  Parent hash    : %s", block.ParentHash().Hex())
	t.Logf("  Timestamp      : %d", block.Time())
	t.Logf("  Tx count       : %d", len(block.Transactions()))
	t.Logf("  Gas used/limit : %d / %d", block.GasUsed(), block.GasLimit())
	t.Logf("  Base fee       : %s wei", block.BaseFee().String())
	t.Logf("  Miner          : %s", block.Coinbase().Hex())

	// ── Step 2: Filter txs from target address ────────────────────────────────
	signer := types.LatestSignerForChainID(new(big.Int).SetInt64(1))
	var targetTxs []*types.Transaction
	for _, tx := range block.Transactions() {
		from, err := types.Sender(signer, tx)
		if err != nil {
			continue
		}
		if from == targetAddr {
			targetTxs = append(targetTxs, tx)
		}
	}

	t.Logf("")
	if len(targetTxs) == 0 {
		t.Logf("  (no transactions directly from target address in this block — address likely acts as a topic)")
	} else {
		t.Logf("  Found %d transaction(s) from target address", len(targetTxs))
	}

	// ── Step 3: For each tx, get receipt + decode all logs ────────────────────
	var allPoolIDs []common.Hash

	for i, tx := range targetTxs {
		receipt, err := client.TransactionReceipt(ctx, tx.Hash())
		if err != nil {
			t.Errorf("tx receipt %s: %v", tx.Hash().Hex(), err)
			continue
		}

		logTxDetails(t, tx, receipt, i)

		if len(receipt.Logs) == 0 {
			t.Logf("  (no logs emitted)")
			continue
		}

		t.Logf("")
		t.Logf("  ── Logs (%d total) ──────────────────────────────────────────────────", len(receipt.Logs))
		for li, lg := range receipt.Logs {
			t.Logf("")
			t.Logf("  Log #%d  address=%s  logIndex=%d", li, lg.Address.Hex(), lg.Index)
			t.Logf("    Topics (%d):", len(lg.Topics))
			for ti, topic := range lg.Topics {
				t.Logf("      [%d] %s", ti, topic.Hex())
			}
			t.Logf("    Data   : %d bytes  0x%s", len(lg.Data), hex.EncodeToString(lg.Data))

			// ── V4 PoolManager events ──────────────────────────────────────────
			if lg.Address == poolManager {
				line, fmtErr := v4FormatLog(&parsedABI, *lg, eventNames, &mu, poolKeys, ctx, client, syms)
				if fmtErr != nil {
					t.Logf("    [V4 decode error] %v", fmtErr)
				} else if line != "" {
					t.Logf("    [V4 decoded] %s", line)
				}
				// Collect pool IDs from Initialize events for later FetchPoolInfo
				if len(lg.Topics) > 0 {
					if name, ok := eventNames[lg.Topics[0]]; ok && name == "Initialize" && len(lg.Topics) > 1 {
						allPoolIDs = append(allPoolIDs, lg.Topics[1])
						t.Logf("    ✓ Pool ID collected: %s", lg.Topics[1].Hex())
					}
				}
				continue
			}

			// ── ERC-721 Transfer (NFT mint / transfer) ─────────────────────────
			if len(lg.Topics) > 0 && lg.Topics[0].Hex() == erc721TransferSig {
				decodeERC721Transfer(t, lg)
				continue
			}

			// ── IncreaseLiquidity (V3-style; may appear on V4 PositionManager) ─
			if len(lg.Topics) > 0 && lg.Topics[0].Hex() == increaseLiqSig {
				decodeIncreaseLiquidity(t, lg)
				continue
			}

			// ── Unknown event — show topic0 for manual lookup ─────────────────
			if len(lg.Topics) > 0 {
				t.Logf("    [unknown event sig] topic0=%s  emitter=%s",
					lg.Topics[0].Hex(), lg.Address.Hex())
			}
		}
		hr(t)
	}

	// ── Step 4: Broad PoolManager scan + client-side address substring match ──
	// Fetch every PoolManager event in the block (no topic filter), then check
	// each topic hex string for the address as a substring. Addresses are
	// ABI-encoded as 32-byte left-zero-padded values, so the 40-char hex of the
	// address (without 0x) will always appear verbatim inside the topic string.
	section(t, fmt.Sprintf("PoolManager scan + address substring match · block %d", v4TestBlock))

	// needle is the lowercase address without 0x prefix.
	needle := strings.ToLower(strings.TrimPrefix(targetAddr.Hex(), "0x"))
	t.Logf("  searching topic hashes for substring: %q", needle)

	allSigs := make([]common.Hash, 0, len(parsedABI.Events))
	for _, ev := range parsedABI.Events {
		allSigs = append(allSigs, ev.ID)
	}
	allPMLogs, pmErr := client.FilterLogs(ctx, ethereum.FilterQuery{
		FromBlock: new(big.Int).SetUint64(v4TestBlock),
		ToBlock:   new(big.Int).SetUint64(v4TestBlock),
		Addresses: []common.Address{poolManager},
		Topics:    [][]common.Hash{allSigs},
	})
	if pmErr != nil {
		t.Logf("  FilterLogs error: %v", pmErr)
	} else {
		t.Logf("  %d PoolManager event(s) in block — scanning topics…", len(allPMLogs))

		// Track receipts already printed.
		printedReceipts := map[common.Hash]bool{}

		for li, lg := range allPMLogs {
			// Client-side: does any topic contain the address substring?
			matchedPos := -1
			for ti, topic := range lg.Topics {
				if strings.Contains(strings.ToLower(topic.Hex()), needle) {
					matchedPos = ti
					break
				}
			}

			t.Logf("")
			if matchedPos >= 0 {
				t.Logf("  ★ MATCH [%d]  tx=%s  logIndex=%d  address in topic[%d]",
					li, lg.TxHash.Hex(), lg.Index, matchedPos)
			} else {
				t.Logf("  event  [%d]  tx=%s  logIndex=%d",
					li, lg.TxHash.Hex(), lg.Index)
			}

			// Decode the V4 event.
			line, fmtErr := v4FormatLog(&parsedABI, lg, eventNames, &mu, poolKeys, ctx, client, syms)
			if fmtErr != nil {
				t.Logf("    [V4 decode error] %v", fmtErr)
			} else if line != "" {
				t.Logf("    [V4] %s", line)
			}

			// Print all topics, marking any that contain the address.
			for ti, topic := range lg.Topics {
				marker := "     "
				if ti == matchedPos {
					marker = "  -> "
				}
				t.Logf("  %stopic[%d]: %s", marker, ti, topic.Hex())
			}
			if len(lg.Data) > 0 {
				t.Logf("    data: 0x%s", hex.EncodeToString(lg.Data))
			}

			// Collect pool IDs.
			if len(lg.Topics) > 1 {
				if name, ok := eventNames[lg.Topics[0]]; ok && name == "Initialize" {
					allPoolIDs = appendPoolID(allPoolIDs, lg.Topics[1])
				}
			}

			// Full tx + receipt for every event, deduplicated by tx hash.
			if !printedReceipts[lg.TxHash] {
				printedReceipts[lg.TxHash] = true
				t.Logf("")
				t.Logf("    ── Full transaction ─────────────────────────────────────")

				// Look up the tx from the already-fetched block rather than a
				// separate TransactionByHash RPC call (some providers return
				// "not found" for eth_getTransactionByHash on archived blocks).
				var matchTx *types.Transaction
				for _, btx := range block.Transactions() {
					if btx.Hash() == lg.TxHash {
						matchTx = btx
						break
					}
				}
				if matchTx == nil {
					t.Logf("    (tx hash %s not found in block %d)", lg.TxHash.Hex(), v4TestBlock)
				} else {
					from, _ := types.Sender(signer, matchTx)
					t.Logf("    from      : %s", from.Hex())
					t.Logf("    to        : %s", addrOrCreate(matchTx.To()))
					t.Logf("    type      : %d", matchTx.Type())
					t.Logf("    nonce     : %d", matchTx.Nonce())
					t.Logf("    value     : %s wei", matchTx.Value().String())
					t.Logf("    gas limit : %d", matchTx.Gas())
					t.Logf("    gas price : %s wei", matchTx.GasPrice().String())
					if matchTx.Type() >= 2 {
						t.Logf("    tip cap   : %s wei", matchTx.GasTipCap().String())
						t.Logf("    fee cap   : %s wei", matchTx.GasFeeCap().String())
					}
					t.Logf("    data size : %d bytes", len(matchTx.Data()))
					if len(matchTx.Data()) >= 4 {
						t.Logf("    selector  : 0x%s", hex.EncodeToString(matchTx.Data()[:4]))
					}
				}

				rcpt, rcptErr := client.TransactionReceipt(ctx, lg.TxHash)
				if rcptErr != nil {
					t.Logf("    receipt fetch error: %v", rcptErr)
				} else {
					statusStr := "FAILED"
					if rcpt.Status == 1 {
						statusStr = "SUCCESS"
					}
					t.Logf("    status    : %s", statusStr)
					t.Logf("    gas used  : %d", rcpt.GasUsed)
					t.Logf("    log count : %d", len(rcpt.Logs))
					t.Logf("")
					t.Logf("    ── All logs in this receipt (%d) ────────────────────", len(rcpt.Logs))
					for rli, rlg := range rcpt.Logs {
						t.Logf("")
						t.Logf("      log [%d/%d]  emitter=%s  logIndex=%d",
							rli+1, len(rcpt.Logs), rlg.Address.Hex(), rlg.Index)
						t.Logf("      topics (%d):", len(rlg.Topics))
						for ti, topic := range rlg.Topics {
							mark := "        "
							if strings.Contains(strings.ToLower(topic.Hex()), needle) {
								mark = "     -> "
							}
							t.Logf("      %s[%d] %s", mark, ti, topic.Hex())
						}
						if len(rlg.Data) > 0 {
							t.Logf("      data  : 0x%s", hex.EncodeToString(rlg.Data))
						}
						if rlg.Address == poolManager {
							rline, rErr := v4FormatLog(&parsedABI, *rlg, eventNames, &mu, poolKeys, ctx, client, syms)
							if rErr != nil {
								t.Logf("      [V4 decode error] %v", rErr)
							} else if rline != "" {
								t.Logf("      [V4] %s", rline)
							}
						} else if len(rlg.Topics) > 0 && rlg.Topics[0] == common.HexToHash(erc721TransferSig) {
							decodeERC721Transfer(t, rlg)
						} else if len(rlg.Topics) > 0 && rlg.Topics[0] == common.HexToHash(increaseLiqSig) {
							decodeIncreaseLiquidity(t, rlg)
						}
					}
				}
			}
		}

		if len(printedReceipts) == 0 {
			t.Logf("  (address not found in any PoolManager topic at block %d)", v4TestBlock)
		}
	}

	// ── Step 5: Full broad PoolManager scan (all events, for reference) ───────
	section(t, fmt.Sprintf("Broad PoolManager scan at block %d (all senders)", v4TestBlock))

	pmLogs, err := client.FilterLogs(ctx, ethereum.FilterQuery{
		FromBlock: new(big.Int).SetUint64(v4TestBlock),
		ToBlock:   new(big.Int).SetUint64(v4TestBlock),
		Addresses: []common.Address{poolManager},
		Topics:    [][]common.Hash{allSigs},
	})
	if err != nil {
		t.Logf("  FilterLogs (PoolManager): %v", err)
	} else {
		t.Logf("  %d PoolManager event(s) emitted in this block", len(pmLogs))
		for li, lg := range pmLogs {
			line, fmtErr := v4FormatLog(&parsedABI, lg, eventNames, &mu, poolKeys, ctx, client, syms)
			if fmtErr != nil {
				t.Logf("  [%d] decode error: %v", li, fmtErr)
			} else if line != "" {
				t.Logf("  [%d] %s", li, line)
			}
			// Collect pool IDs from Initialize events
			if len(lg.Topics) > 1 {
				if name, ok := eventNames[lg.Topics[0]]; ok && name == "Initialize" {
					allPoolIDs = appendPoolID(allPoolIDs, lg.Topics[1])
				}
			}
		}
	}

	// ── Step 6: PositionManager — all events (reference) ─────────────────────
	posManager := common.HexToAddress(v4PositionManagerAddress)
	section(t, fmt.Sprintf("PositionManager all events · block %d · %s", v4TestBlock, posManager.Hex()))

	erc721Sig := common.HexToHash(erc721TransferSig)
	incLiqSig := common.HexToHash(increaseLiqSig)

	allPosLogs, posErr := client.FilterLogs(ctx, ethereum.FilterQuery{
		FromBlock: new(big.Int).SetUint64(v4TestBlock),
		ToBlock:   new(big.Int).SetUint64(v4TestBlock),
		Addresses: []common.Address{posManager},
	})
	if posErr != nil {
		t.Logf("  FilterLogs error: %v", posErr)
	} else {
		t.Logf("  %d PositionManager event(s) in block", len(allPosLogs))
		for li, lg := range allPosLogs {
			t.Logf("")
			t.Logf("  event [%d]  tx=%s  logIndex=%d", li, lg.TxHash.Hex(), lg.Index)
			switch {
			case len(lg.Topics) > 0 && lg.Topics[0] == erc721Sig:
				decodeERC721Transfer(t, &lg)
			case len(lg.Topics) > 0 && lg.Topics[0] == incLiqSig:
				decodeIncreaseLiquidity(t, &lg)
			}
			t.Logf("  topics (%d):", len(lg.Topics))
			for ti, topic := range lg.Topics {
				t.Logf("          [%d] %s", ti, topic.Hex())
			}
			if len(lg.Data) > 0 {
				t.Logf("  data  : 0x%s", hex.EncodeToString(lg.Data))
			}
		}
	}

	// ── Step 7: PositionManager — 'from' field (topic[1]) match ──────────────
	// ERC-721 Transfer layout: topic[0]=sig, topic[1]=from, topic[2]=to, topic[3]=tokenId.
	// Pinning the target address at topic[1] is the definitive check: if any
	// result is returned the PositionManager portion of the test passes.
	section(t, fmt.Sprintf("PositionManager 'from' field search · block %d", v4TestBlock))

	addrTopic := common.BytesToHash(targetAddr.Bytes())
	t.Logf("  topic[1] filter : %s", addrTopic.Hex())

	fromLogs, fromErr := client.FilterLogs(ctx, ethereum.FilterQuery{
		FromBlock: new(big.Int).SetUint64(v4TestBlock),
		ToBlock:   new(big.Int).SetUint64(v4TestBlock),
		Addresses: []common.Address{posManager},
		Topics:    [][]common.Hash{nil, {addrTopic}},
	})
	if fromErr != nil {
		t.Errorf("  FilterLogs error: %v", fromErr)
	} else if len(fromLogs) == 0 {
		t.Logf("  RESULT: no events found with target address at topic[1]")
	} else {
		t.Logf("  PASS: %d event(s) found with target address at topic[1]", len(fromLogs))
		for li, lg := range fromLogs {
			t.Logf("")
			t.Logf("  [%d]  tx=%s  logIndex=%d  emitter=%s",
				li, lg.TxHash.Hex(), lg.Index, lg.Address.Hex())
			t.Logf("  topics (%d):", len(lg.Topics))
			for ti, topic := range lg.Topics {
				mark := "        "
				if ti == 1 {
					mark = "     -> "
				}
				t.Logf("  %s[%d] %s", mark, ti, topic.Hex())
			}
			if len(lg.Data) > 0 {
				t.Logf("  data  : 0x%s", hex.EncodeToString(lg.Data))
			}
			switch {
			case len(lg.Topics) > 0 && lg.Topics[0] == erc721Sig:
				decodeERC721Transfer(t, &lg)
			case len(lg.Topics) > 0 && lg.Topics[0] == incLiqSig:
				decodeIncreaseLiquidity(t, &lg)
			}
		}
	}

	// ── Step 8: FetchPoolInfo for every Initialize event found ────────────────
	if len(allPoolIDs) > 0 {
		section(t, fmt.Sprintf("Live pool state (StateView)  ·  %d pool(s)", len(allPoolIDs)))
		for _, poolID := range allPoolIDs {
			t.Logf("  Pool ID : %s", poolID.Hex())
			info, fetchErr := FetchPoolInfo(rpcURL, poolID)
			if fetchErr != nil {
				t.Logf("    FetchPoolInfo error: %v", fetchErr)
			} else {
				t.Logf("    sqrtPriceX96 : %s", info.SqrtPriceX96)
				t.Logf("    tick         : %d", info.Tick)
				t.Logf("    protocolFee  : %d", info.ProtocolFee)
				t.Logf("    lpFee        : %d", info.LpFee)
				t.Logf("    liquidity    : %s", info.Liquidity)
			}

			keyInfo, keyErr := FetchPoolKey(rpcURL, poolID)
			if keyErr != nil {
				t.Logf("    FetchPoolKey error: %v", keyErr)
			} else {
				t.Logf("    currency0    : %s", keyInfo.Currency0)
				t.Logf("    currency1    : %s", keyInfo.Currency1)
				t.Logf("    fee          : %d (%.4f%%)", keyInfo.Fee, float64(keyInfo.Fee)/1e4)
				t.Logf("    tickSpacing  : %d", keyInfo.TickSpacing)
				t.Logf("    hooks        : %s", keyInfo.Hooks)
			}
			hr(t)
		}
	}

	section(t, "Done")
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func appendPoolID(ids []common.Hash, id common.Hash) []common.Hash {
	for _, existing := range ids {
		if existing == id {
			return ids
		}
	}
	return append(ids, id)
}

// ── Decoders ──────────────────────────────────────────────────────────────────

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
