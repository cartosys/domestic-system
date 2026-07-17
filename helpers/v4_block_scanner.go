package helpers

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"

	"charm-wallet-tui/styles"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

var (
	bsAccent  = lipgloss.NewStyle().Foreground(styles.CAccent)
	bsAccent2 = lipgloss.NewStyle().Foreground(styles.CAccent2)
	bsWarn    = lipgloss.NewStyle().Foreground(styles.CWarn)
	bsError   = lipgloss.NewStyle().Foreground(styles.CError)
	bsMuted   = lipgloss.NewStyle().Foreground(styles.CMuted)
	bsBorder  = lipgloss.NewStyle().Foreground(styles.CBorder)
)

func bsLabel(s string) string   { return bsAccent2.Render(s) }
func bsValue(s string) string   { return bsAccent.Render(s) }
func bsSection(s string) string { return FadeString(s, "#7EE787", "#82CFFD") }
func bsDivider() string {
	return bsBorder.Render("────────────────────────────────────────────────────────")
}
func bsHeader(s string) string {
	bar := bsBorder.Render("════════════════════════════════════════════════════════")
	return bar + "\n" + bsSection("  "+s) + "\n" + bar
}

// bsHyperBlock returns a FadeString-coloured block number hyperlinked to Etherscan.
func bsHyperBlock(n uint64) string {
	s := fmt.Sprintf("%d", n)
	display := FadeString(s, "#FFA657", "#EDFF82")
	return ansi.SetHyperlink(fmt.Sprintf("https://etherscan.io/block/%d", n)) + display + ansi.ResetHyperlink()
}


// bsHyperHash returns a plain hash hyperlinked to Etherscan (block hash, etc.).
func bsHyperHash(h common.Hash, urlPath string) string {
	s := h.Hex()
	short := s[:10] + "…" + s[len(s)-6:]
	display := FadeString(short, "#874BFD", "#79C0FF")
	return ansi.SetHyperlink("https://etherscan.io/"+urlPath+h.Hex()) + display + ansi.ResetHyperlink()
}

// ── V4BlockScanner ────────────────────────────────────────────────────────────

// V4BlockScanner performs a one-shot forensic scan of a single Ethereum block,
// looking for Uniswap V4 Initialize (pool creation) and ModifyLiquidity / NFT
// Transfer (LP position mint) events. Results are streamed as styled lines
// suitable for the TUI log panel.
type V4BlockScanner struct {
	lines  chan string
	cancel context.CancelFunc
}

// NewV4BlockScanner creates a new V4BlockScanner ready to be started.
func NewV4BlockScanner() *V4BlockScanner {
	return &V4BlockScanner{lines: make(chan string, 1024)}
}

// Start dials rpcURL and begins scanning block blockNum for transactions sent
// by fromAddr. Lines are available via Lines() and the channel is closed when
// the scan completes or an error occurs.
func (s *V4BlockScanner) Start(rpcURL string, blockNum uint64, fromAddr common.Address) {
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	go s.run(ctx, rpcURL, blockNum, fromAddr)
}

// Stop cancels an in-progress scan.
func (s *V4BlockScanner) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
}

// Lines returns the read-only channel of formatted log lines.
// The channel is closed when the scan finishes.
func (s *V4BlockScanner) Lines() <-chan string {
	return s.lines
}

func (s *V4BlockScanner) emit(line string) {
	select {
	case s.lines <- line:
	default:
	}
}

func (s *V4BlockScanner) emitErr(msg string) {
	s.emit(bsError.Render("[BlockScan] ERROR: ") + bsMuted.Render(msg))
}

// ── run ───────────────────────────────────────────────────────────────────────

func (s *V4BlockScanner) run(ctx context.Context, rpcURL string, blockNum uint64, fromAddr common.Address) {
	defer close(s.lines)

	dialCtx, dialCancel := context.WithTimeout(ctx, 12*time.Second)
	defer dialCancel()

	client, err := ethclient.DialContext(dialCtx, rpcURL)
	if err != nil {
		s.emitErr(fmt.Sprintf("dial RPC: %v", err))
		return
	}
	defer client.Close()

	// Parse V4 ABI once.
	parsedABI, err := abi.JSON(strings.NewReader(poolManagerEventsABI))
	if err != nil {
		s.emitErr(fmt.Sprintf("parse ABI: %v", err))
		return
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

	addrs := addressesForClient(ctx, client)
	poolManager := addrs.V4PoolManager
	posManager := addrs.V4PositionManager
	erc721Sig := common.HexToHash("0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef")
	incLiqSig := common.HexToHash("0x3067048beee31b25b2f1681f88dac838c8bba36af25bfb2b7cf7473a5847e35f")

	// ── Block header ──────────────────────────────────────────────────────────
	s.emit(bsHeader(fmt.Sprintf("Block Scan · %s · from %s",
		bsHyperBlock(blockNum), HyperAddr(fromAddr))))

	blockCtx, blockCancel := context.WithTimeout(ctx, 20*time.Second)
	defer blockCancel()

	block, err := client.BlockByNumber(blockCtx, new(big.Int).SetUint64(blockNum))
	if err != nil {
		s.emitErr(fmt.Sprintf("fetch block: %v", err))
		return
	}

	s.emit(fmt.Sprintf("  %s %s", bsLabel("block hash  :"), bsHyperHash(block.Hash(), "block/")))
	s.emit(fmt.Sprintf("  %s %s", bsLabel("parent hash :"), bsHyperHash(block.ParentHash(), "block/")))
	s.emit(fmt.Sprintf("  %s %s", bsLabel("timestamp   :"), bsValue(fmt.Sprintf("%d", block.Time()))))
	s.emit(fmt.Sprintf("  %s %s", bsLabel("txs in block:"), bsValue(fmt.Sprintf("%d", len(block.Transactions())))))
	s.emit(fmt.Sprintf("  %s %s / %s", bsLabel("gas used    :"),
		bsValue(fmt.Sprintf("%d", block.GasUsed())),
		bsValue(fmt.Sprintf("%d", block.GasLimit()))))
	s.emit(fmt.Sprintf("  %s %s wei", bsLabel("base fee    :"), bsValue(block.BaseFee().String())))
	s.emit(fmt.Sprintf("  %s %s", bsLabel("miner       :"), HyperAddr(block.Coinbase())))

	// ── Find txs from target address ──────────────────────────────────────────
	signer := types.LatestSignerForChainID(new(big.Int).SetInt64(1))
	var targetTxs []*types.Transaction
	for _, tx := range block.Transactions() {
		from, sigErr := types.Sender(signer, tx)
		if sigErr != nil {
			continue
		}
		if from == fromAddr {
			targetTxs = append(targetTxs, tx)
		}
	}

	if len(targetTxs) == 0 {
		s.emit(bsWarn.Render(fmt.Sprintf(
			"  [BlockScan] no transactions from %s in block %d", fromAddr.Hex(), blockNum)))
	} else {
		s.emit(fmt.Sprintf("  %s %s",
			bsLabel("target txs  :"), bsValue(fmt.Sprintf("%d", len(targetTxs)))))
	}

	var allPoolIDs []common.Hash

	// ── Per-transaction detail ─────────────────────────────────────────────────
	for i, tx := range targetTxs {
		select {
		case <-ctx.Done():
			return
		default:
		}

		s.emit(bsDivider())
		s.emit(bsSection(fmt.Sprintf("  Transaction #%d", i+1)))

		s.emit(fmt.Sprintf("  %s %s", bsLabel("hash       :"), HyperTxHash(tx.Hash())))
		s.emit(fmt.Sprintf("  %s %s", bsLabel("type       :"),
			bsValue(fmt.Sprintf("%d", tx.Type()))))
		s.emit(fmt.Sprintf("  %s %s", bsLabel("nonce      :"),
			bsValue(fmt.Sprintf("%d", tx.Nonce()))))
		s.emit(fmt.Sprintf("  %s %s", bsLabel("to         :"), HyperAddr(addrOrZero(tx.To()))))
		s.emit(fmt.Sprintf("  %s %s wei", bsLabel("value      :"), bsValue(tx.Value().String())))
		s.emit(fmt.Sprintf("  %s %s", bsLabel("gas limit  :"), bsValue(fmt.Sprintf("%d", tx.Gas()))))
		s.emit(fmt.Sprintf("  %s %s wei", bsLabel("gas price  :"), bsValue(tx.GasPrice().String())))
		if tx.Type() >= 2 {
			s.emit(fmt.Sprintf("  %s %s wei", bsLabel("tip cap    :"), bsValue(tx.GasTipCap().String())))
			s.emit(fmt.Sprintf("  %s %s wei", bsLabel("fee cap    :"), bsValue(tx.GasFeeCap().String())))
		}
		s.emit(fmt.Sprintf("  %s %s bytes", bsLabel("data size  :"), bsValue(fmt.Sprintf("%d", len(tx.Data())))))
		if len(tx.Data()) >= 4 {
			sel := hex.EncodeToString(tx.Data()[:4])
			s.emit(fmt.Sprintf("  %s %s",
				bsLabel("selector   :"),
				FadeString("0x"+sel, "#874BFD", "#79C0FF")))
		}
		if len(tx.Data()) > 0 {
			s.emit(fmt.Sprintf("  %s %s",
				bsLabel("calldata   :"),
				bsMuted.Render("0x"+hex.EncodeToString(tx.Data()))))
		}

		rcptCtx, rcptCancel := context.WithTimeout(ctx, 15*time.Second)
		receipt, rcptErr := client.TransactionReceipt(rcptCtx, tx.Hash())
		rcptCancel()
		if rcptErr != nil {
			s.emitErr(fmt.Sprintf("receipt for %s: %v", tx.Hash().Hex(), rcptErr))
			continue
		}

		statusStr := bsError.Render("FAILED")
		if receipt.Status == 1 {
			statusStr = bsAccent.Render("SUCCESS")
		}
		s.emit(fmt.Sprintf("  %s %s", bsLabel("status     :"), statusStr))
		s.emit(fmt.Sprintf("  %s %s  (%s%%)",
			bsLabel("gas used   :"),
			bsValue(fmt.Sprintf("%d", receipt.GasUsed)),
			bsValue(fmt.Sprintf("%.1f", float64(receipt.GasUsed)/float64(tx.Gas())*100))))
		s.emit(fmt.Sprintf("  %s %s", bsLabel("log count  :"),
			bsValue(fmt.Sprintf("%d", len(receipt.Logs)))))
		if receipt.ContractAddress != (common.Address{}) {
			s.emit(fmt.Sprintf("  %s %s",
				bsLabel("contract   :"), HyperAddr(receipt.ContractAddress)))
		}

		// ── Logs ──────────────────────────────────────────────────────────────
		for li, lg := range receipt.Logs {
			select {
			case <-ctx.Done():
				return
			default:
			}

			s.emit(fmt.Sprintf("  %s #%d  %s  logIdx=%s",
				bsAccent2.Render("log"),
				li,
				HyperAddr(lg.Address),
				bsValue(fmt.Sprintf("%d", lg.Index))))

			for ti, topic := range lg.Topics {
				label := bsMuted.Render(fmt.Sprintf("    topic[%d]:", ti))
				s.emit(fmt.Sprintf("%s %s", label, bsMuted.Render(topic.Hex())))
			}
			if len(lg.Data) > 0 {
				s.emit(fmt.Sprintf("    %s %s bytes  %s",
					bsMuted.Render("data  :"),
					bsMuted.Render(fmt.Sprintf("%d", len(lg.Data))),
					bsMuted.Render("0x"+hex.EncodeToString(lg.Data))))
			}

			// V4 PoolManager
			if lg.Address == poolManager {
				line, fmtErr := v4FormatLog(&parsedABI, *lg, eventNames, &mu, poolKeys, ctx, client, syms)
				if fmtErr != nil {
					s.emit(fmt.Sprintf("    %s %v", bsError.Render("[V4 err]"), fmtErr))
				} else if line != "" {
					s.emit("    " + line)
				}
				if len(lg.Topics) > 1 {
					if name, ok := eventNames[lg.Topics[0]]; ok && name == "Initialize" {
						allPoolIDs = appendUniq(allPoolIDs, lg.Topics[1])
					}
				}
				continue
			}

			// ERC-721 Transfer / Mint
			if len(lg.Topics) > 0 && lg.Topics[0] == erc721Sig {
				s.emit("    " + decodeERC721Line(lg))
				continue
			}

			// IncreaseLiquidity
			if len(lg.Topics) > 0 && lg.Topics[0] == incLiqSig {
				s.emit("    " + decodeIncLiqLine(lg))
				continue
			}

			// Unknown
			if len(lg.Topics) > 0 {
				s.emit(fmt.Sprintf("    %s topic0=%s  emitter=%s",
					bsWarn.Render("[unknown event]"),
					bsMuted.Render(lg.Topics[0].Hex()),
					HyperAddr(lg.Address)))
			}
		}
	}

	// ── Broad PoolManager scan ────────────────────────────────────────────────
	select {
	case <-ctx.Done():
		return
	default:
	}

	s.emit(bsHeader(fmt.Sprintf("PoolManager · all senders · block %s",
		bsHyperBlock(blockNum))))

	allSigs := make([]common.Hash, 0, len(parsedABI.Events))
	for _, ev := range parsedABI.Events {
		allSigs = append(allSigs, ev.ID)
	}
	pmCtx, pmCancel := context.WithTimeout(ctx, 20*time.Second)
	pmLogs, pmErr := client.FilterLogs(pmCtx, ethereum.FilterQuery{
		FromBlock: new(big.Int).SetUint64(blockNum),
		ToBlock:   new(big.Int).SetUint64(blockNum),
		Addresses: []common.Address{poolManager},
		Topics:    [][]common.Hash{allSigs},
	})
	pmCancel()
	if pmErr != nil {
		s.emitErr(fmt.Sprintf("FilterLogs PoolManager: %v", pmErr))
	} else {
		s.emit(fmt.Sprintf("  %s %s",
			bsLabel("events :"), bsValue(fmt.Sprintf("%d", len(pmLogs)))))
		for li, lg := range pmLogs {
			select {
			case <-ctx.Done():
				return
			default:
			}
			line, fmtErr := v4FormatLog(&parsedABI, lg, eventNames, &mu, poolKeys, ctx, client, syms)
			if fmtErr != nil {
				s.emit(fmt.Sprintf("  [%d] %s %v", li, bsError.Render("decode err:"), fmtErr))
			} else if line != "" {
				s.emit(fmt.Sprintf("  [%d] %s", li, line))
			}
			if len(lg.Topics) > 1 {
				if name, ok := eventNames[lg.Topics[0]]; ok && name == "Initialize" {
					allPoolIDs = appendUniq(allPoolIDs, lg.Topics[1])
				}
			}
		}
	}

	// ── PositionManager scan ──────────────────────────────────────────────────
	select {
	case <-ctx.Done():
		return
	default:
	}

	s.emit(bsHeader(fmt.Sprintf("PositionManager · ERC-721 · block %s",
		bsHyperBlock(blockNum))))
	s.emit(fmt.Sprintf("  %s %s", bsLabel("address :"), HyperAddr(posManager)))

	posCtx, posCancel := context.WithTimeout(ctx, 20*time.Second)
	posLogs, posErr := client.FilterLogs(posCtx, ethereum.FilterQuery{
		FromBlock: new(big.Int).SetUint64(blockNum),
		ToBlock:   new(big.Int).SetUint64(blockNum),
		Addresses: []common.Address{posManager},
		Topics:    [][]common.Hash{{erc721Sig, incLiqSig}},
	})
	posCancel()
	if posErr != nil {
		s.emitErr(fmt.Sprintf("FilterLogs PositionManager: %v", posErr))
	} else {
		s.emit(fmt.Sprintf("  %s %s", bsLabel("events :"), bsValue(fmt.Sprintf("%d", len(posLogs)))))
		for li, lg := range posLogs {
			select {
			case <-ctx.Done():
				return
			default:
			}
			s.emit(fmt.Sprintf("  [%d] tx=%s  logIdx=%s",
				li, HyperTxHash(lg.TxHash), bsValue(fmt.Sprintf("%d", lg.Index))))
			switch {
			case len(lg.Topics) > 0 && lg.Topics[0] == erc721Sig:
				s.emit("       " + decodeERC721Line(&lg))
			case len(lg.Topics) > 0 && lg.Topics[0] == incLiqSig:
				s.emit("       " + decodeIncLiqLine(&lg))
			default:
				if len(lg.Topics) > 0 {
					s.emit(fmt.Sprintf("       %s %s", bsWarn.Render("[unknown]"), lg.Topics[0].Hex()))
				}
			}
		}
	}

	// ── Live pool state ───────────────────────────────────────────────────────
	if len(allPoolIDs) > 0 {
		select {
		case <-ctx.Done():
			return
		default:
		}

		s.emit(bsHeader(fmt.Sprintf("Live Pool State · %d pool(s)", len(allPoolIDs))))

		for _, poolID := range allPoolIDs {
			select {
			case <-ctx.Done():
				return
			default:
			}
			poolIDShort := poolID.Hex()[:10] + "…" + poolID.Hex()[len(poolID.Hex())-6:]
			s.emit(fmt.Sprintf("  %s %s",
				bsLabel("pool ID :"),
				FadeString(poolIDShort, "#7EE787", "#82CFFD")))
			s.emit(fmt.Sprintf("           %s", bsMuted.Render(poolID.Hex())))

			info, infoErr := FetchPoolInfo(rpcURL, poolID)
			if infoErr != nil {
				s.emit(fmt.Sprintf("  %s %v", bsError.Render("FetchPoolInfo:"), infoErr))
			} else {
				s.emit(fmt.Sprintf("  %s %s", bsLabel("sqrtPriceX96 :"), bsValue(info.SqrtPriceX96)))
				s.emit(fmt.Sprintf("  %s %s", bsLabel("tick         :"), bsValue(fmt.Sprintf("%d", info.Tick))))
				s.emit(fmt.Sprintf("  %s %s", bsLabel("protocolFee  :"), bsValue(fmt.Sprintf("%d", info.ProtocolFee))))
				s.emit(fmt.Sprintf("  %s %s", bsLabel("lpFee        :"), bsValue(fmt.Sprintf("%d", info.LpFee))))
				s.emit(fmt.Sprintf("  %s %s", bsLabel("liquidity    :"), bsValue(info.Liquidity)))
			}

			key, keyErr := FetchPoolKey(rpcURL, poolID)
			if keyErr != nil {
				s.emit(fmt.Sprintf("  %s %v", bsError.Render("FetchPoolKey :"), keyErr))
			} else {
				c0Addr := common.HexToAddress(key.Currency0)
				c1Addr := common.HexToAddress(key.Currency1)
				c0Label := HyperAddr(c0Addr)
				c1Label := HyperAddr(c1Addr)
				if key.Currency0 == "NATIVE" {
					c0Label = bsAccent.Render("NATIVE (ETH)")
				}
				if key.Currency1 == "NATIVE" {
					c1Label = bsAccent.Render("NATIVE (ETH)")
				}
				s.emit(fmt.Sprintf("  %s %s", bsLabel("currency0    :"), c0Label))
				s.emit(fmt.Sprintf("  %s %s", bsLabel("currency1    :"), c1Label))
				s.emit(fmt.Sprintf("  %s %s  %s",
					bsLabel("fee          :"),
					bsValue(fmt.Sprintf("%d", key.Fee)),
					bsMuted.Render(fmt.Sprintf("(%.4f%%)", float64(key.Fee)/1e4))))
				s.emit(fmt.Sprintf("  %s %s", bsLabel("tickSpacing  :"), bsValue(fmt.Sprintf("%d", key.TickSpacing))))
				hooksAddr := common.HexToAddress(key.Hooks)
				s.emit(fmt.Sprintf("  %s %s", bsLabel("hooks        :"), HyperAddr(hooksAddr)))
			}
			s.emit(bsDivider())
		}
	}

	s.emit(bsHeader("Block Scan Complete"))
}

// ── Internal helpers ──────────────────────────────────────────────────────────

func addrOrZero(a *common.Address) common.Address {
	if a == nil {
		return common.Address{}
	}
	return *a
}

func appendUniq(ids []common.Hash, id common.Hash) []common.Hash {
	for _, existing := range ids {
		if existing == id {
			return ids
		}
	}
	return append(ids, id)
}

// decodeERC721Line formats an ERC-721 Transfer event line.
// topic layout: [sig, from (indexed), to (indexed), tokenId (indexed)]
func decodeERC721Line(lg *types.Log) string {
	if len(lg.Topics) < 4 {
		return bsWarn.Render(fmt.Sprintf("[ERC-721 Transfer] too few topics (%d)", len(lg.Topics)))
	}
	from := common.BytesToAddress(lg.Topics[1].Bytes()[12:])
	to := common.BytesToAddress(lg.Topics[2].Bytes()[12:])
	tokenID := new(big.Int).SetBytes(lg.Topics[3].Bytes())
	label := "Transfer"
	if (from == common.Address{}) {
		label = "Mint    "
	}
	return fmt.Sprintf("[ERC-721 %s]  contract=%s  from=%s  to=%s  tokenId=%s",
		bsAccent.Render(label),
		HyperAddr(lg.Address),
		HyperAddr(from),
		HyperAddr(to),
		bsValue(tokenID.String()),
	)
}

// decodeIncLiqLine formats an IncreaseLiquidity event line.
// topic layout: [sig, tokenId (indexed)]
// data: liquidity uint128, amount0 uint128, amount1 uint128
func decodeIncLiqLine(lg *types.Log) string {
	if len(lg.Topics) < 2 {
		return bsWarn.Render(fmt.Sprintf("[IncreaseLiquidity] too few topics (%d)", len(lg.Topics)))
	}
	tokenID := new(big.Int).SetBytes(lg.Topics[1].Bytes())
	if len(lg.Data) < 96 {
		return fmt.Sprintf("[IncreaseLiquidity]  tokenId=%s  %s",
			bsValue(tokenID.String()),
			bsWarn.Render(fmt.Sprintf("(data too short: %d bytes)", len(lg.Data))))
	}
	liq := new(big.Int).SetBytes(lg.Data[0:32])
	amt0 := new(big.Int).SetBytes(lg.Data[32:64])
	amt1 := new(big.Int).SetBytes(lg.Data[64:96])
	return fmt.Sprintf("[IncreaseLiquidity]  contract=%s  tokenId=%s  liquidity=%s  amount0=%s  amount1=%s",
		HyperAddr(lg.Address),
		bsValue(tokenID.String()),
		bsValue(liq.String()),
		bsValue(amt0.String()),
		bsValue(amt1.String()),
	)
}
