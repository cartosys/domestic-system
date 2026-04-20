package main

import (
	"context"
	"fmt"
	"image"
	"math/big"
	"os/exec"
	"strings"
	"time"

	"charm-wallet-tui/config"
	"charm-wallet-tui/helpers"
	"charm-wallet-tui/indexer"
	"charm-wallet-tui/rpc"
	"charm-wallet-tui/store"
	"charm-wallet-tui/views/uniswap"
	"charm-wallet-tui/webcam/capture"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/makiuchi-d/gozxing"
	gozxingqr "github.com/makiuchi-d/gozxing/qrcode"
)

// -------------------- COMMAND FUNCTIONS --------------------
// Functions that return tea.Cmd for async operations

// connectRPC establishes an RPC connection to the Ethereum node
func connectRPC(url string) tea.Cmd {
	return func() tea.Msg {
		result := rpc.Connect(url)
		return rpcConnectedMsg{client: result.Client, err: result.Error}
	}
}

// initLogViewport initializes the log viewport
func initLogViewport() tea.Cmd {
	return func() tea.Msg {
		return logInitMsg{}
	}
}

// packageTransaction packages an ETH transfer transaction for QR display using EIP-4527 encoding.
func packageTransaction(fromAddr, toAddr string, ethAmount string, rpcURL string) tea.Cmd {
	return func() tea.Msg {
		amountFloat := new(big.Float)
		amountFloat.SetString(ethAmount)
		weiFloat := new(big.Float).Mul(amountFloat, big.NewFloat(1e18))
		amountWei, _ := weiFloat.Int(nil)

		urStr, err := rpc.PackUnsignedTxEIP4527(
			common.HexToAddress(fromAddr),
			common.HexToAddress(toAddr),
			amountWei,
			21000,
			nil,
			rpcURL,
		)
		if err != nil {
			return packageTransactionMsg{err: err}
		}
		summary := fmt.Sprintf("ETH Transfer: %s ETH → %s\n\n%s", ethAmount, toAddr, urStr)
		return packageTransactionMsg{txDisplay: summary, qrData: urStr, format: "EIP-4527", err: nil}
	}
}

// packageSwapTransaction packages a Uniswap V2 swap as a proper EIP-4527 unsigned transaction.
// The calldata is ABI-encoded, the tx is RLP-encoded (EIP-155 signing preimage),
// wrapped in CBOR as an eth-sign-request, and UR-encoded for QR display.
func packageSwapTransaction(fromAddr string, fromToken, toToken uniswap.TokenOption, amountIn string, amountOutMin *big.Int, rpcURL string) tea.Cmd {
	return func() tea.Msg {
		routerAddress := common.HexToAddress("0x7a250d5630B4cF539739dF2C5dAcb4c659F2488D")
		wethAddress := common.HexToAddress("0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2")
		fromAddress := common.HexToAddress(fromAddr)

		// Convert human-readable amountIn to base units
		amountFloat := new(big.Float)
		amountFloat.SetString(amountIn)
		multiplier := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(fromToken.Decimals)), nil))
		amountInBig, _ := new(big.Float).Mul(amountFloat, multiplier).Int(nil)

		outDecimals := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(toToken.Decimals)), nil)
		minOutHuman := new(big.Float).Quo(new(big.Float).SetInt(amountOutMin), new(big.Float).SetInt(outDecimals)).Text('f', 6)

		deadline := int64(time.Now().Unix() + 1200) // 20 min window for signing

		// ABI-encode the Uniswap V2 router calldata
		calldata := buildSwapCalldata(fromToken, toToken, fromAddress, amountInBig, amountOutMin, wethAddress, deadline)

		// ETH→Token sends ETH as value; token swaps send zero value
		txValue := big.NewInt(0)
		if fromToken.IsETH {
			txValue = amountInBig
		}

		// Fetch network params, RLP+CBOR+UR encode
		urStr, err := rpc.PackUnsignedTxEIP4527(fromAddress, routerAddress, txValue, 200000, calldata, rpcURL)
		if err != nil {
			return packageTransactionMsg{err: err}
		}

		// Human-readable summary shown below the QR code
		summary := fmt.Sprintf("Uniswap V2 Swap: %s %s → %s (min %s)\nRouter: %s\n\n%s",
			amountIn, fromToken.Symbol, toToken.Symbol, minOutHuman,
			routerAddress.Hex(),
			urStr,
		)

		return packageTransactionMsg{txDisplay: summary, qrData: urStr, format: "EIP-4527", err: nil}
	}
}

// abiEncodeUint256 ABI-encodes a *big.Int as a 32-byte uint256.
func abiEncodeUint256(v *big.Int) []byte {
	b := make([]byte, 32)
	vb := v.Bytes()
	copy(b[32-len(vb):], vb)
	return b
}

// abiEncodeAddress ABI-encodes a common.Address as a 32-byte padded value.
func abiEncodeAddress(addr common.Address) []byte {
	b := make([]byte, 32)
	copy(b[12:], addr[:])
	return b
}

// buildSwapCalldata builds the ABI-encoded calldata for the appropriate Uniswap V2 swap function.
func buildSwapCalldata(fromToken, toToken uniswap.TokenOption, to common.Address, amountIn, amountOutMin *big.Int, weth common.Address, deadline int64) []byte {
	dl := big.NewInt(deadline)
	if fromToken.IsETH {
		// swapExactETHForTokens(uint256 amountOutMin, address[] path, address to, uint256 deadline)
		// selector: 0x7ff36ab5
		var d []byte
		d = append(d, 0x7f, 0xf3, 0x6a, 0xb5)
		d = append(d, abiEncodeUint256(amountOutMin)...)
		d = append(d, abiEncodeUint256(big.NewInt(128))...) // path offset: 4 params * 32
		d = append(d, abiEncodeAddress(to)...)
		d = append(d, abiEncodeUint256(dl)...)
		d = append(d, abiEncodeUint256(big.NewInt(2))...) // path length
		d = append(d, abiEncodeAddress(weth)...)
		d = append(d, abiEncodeAddress(toToken.Address)...)
		return d
	} else if toToken.IsETH {
		// swapExactTokensForETH(uint256 amountIn, uint256 amountOutMin, address[] path, address to, uint256 deadline)
		// selector: 0x18cbafe5
		var d []byte
		d = append(d, 0x18, 0xcb, 0xaf, 0xe5)
		d = append(d, abiEncodeUint256(amountIn)...)
		d = append(d, abiEncodeUint256(amountOutMin)...)
		d = append(d, abiEncodeUint256(big.NewInt(160))...) // path offset: 5 params * 32
		d = append(d, abiEncodeAddress(to)...)
		d = append(d, abiEncodeUint256(dl)...)
		d = append(d, abiEncodeUint256(big.NewInt(2))...) // path length
		d = append(d, abiEncodeAddress(fromToken.Address)...)
		d = append(d, abiEncodeAddress(weth)...)
		return d
	}
	// swapExactTokensForTokens(uint256 amountIn, uint256 amountOutMin, address[] path, address to, uint256 deadline)
	// selector: 0x38ed1739
	var d []byte
	d = append(d, 0x38, 0xed, 0x17, 0x39)
	d = append(d, abiEncodeUint256(amountIn)...)
	d = append(d, abiEncodeUint256(amountOutMin)...)
	d = append(d, abiEncodeUint256(big.NewInt(160))...) // path offset: 5 params * 32
	d = append(d, abiEncodeAddress(to)...)
	d = append(d, abiEncodeUint256(dl)...)
	d = append(d, abiEncodeUint256(big.NewInt(2))...) // path length
	d = append(d, abiEncodeAddress(fromToken.Address)...)
	d = append(d, abiEncodeAddress(toToken.Address)...)
	return d
}

// loadDetails fetches wallet balance details from the blockchain
func loadDetails(client *rpc.Client, addr common.Address, watch []rpc.WatchedToken) tea.Cmd {
	return func() tea.Msg {
		rpcDetails := rpc.LoadWalletDetails(client, addr, watch)

		// Convert rpc.WalletDetails to our config.WalletDetails type
		d := config.WalletDetails{
			Address:    rpcDetails.Address,
			EthWei:     rpcDetails.EthWei,
			LoadedAt:   rpcDetails.LoadedAt,
			ErrMessage: rpcDetails.ErrMessage,
		}

		// Convert token balances
		for _, t := range rpcDetails.Tokens {
			d.Tokens = append(d.Tokens, config.TokenBalance{
				Symbol:   t.Symbol,
				Decimals: t.Decimals,
				Balance:  t.Balance,
			})
		}

		return detailsLoadedMsg{d: d, err: nil}
	}
}

// copyToClipboard copies text to clipboard
func copyToClipboard(text string) tea.Cmd {
	return func() tea.Msg {
		err := clipboard.WriteAll(text)
		if err == nil {
			return clipboardCopiedMsg{}
		}
		return nil
	}
}

// indexERC20TokensCmd looks up name/symbol/decimals for each address via eth_call
// and stores the results in the erc20_tokens table. Addresses already in the
// table are skipped. Fires erc20TokenIndexedMsg when done (errors are silently
// swallowed so a non-standard token never breaks the caller's flow).
func indexERC20TokensCmd(s *store.Store, rpcURL string, addrs ...common.Address) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		defer cancel()
		for _, addr := range addrs {
			_ = s.EnsureERC20Token(ctx, rpcURL, addr)
		}
		return erc20TokenIndexedMsg{}
	}
}

// copyPoolIDToClipboard copies a V4 pool ID to the clipboard and signals the Pool Info popup.
func copyPoolIDToClipboard(poolID string) tea.Cmd {
	return func() tea.Msg {
		if err := clipboard.WriteAll(poolID); err == nil {
			return poolIDCopiedMsg{}
		}
		return nil
	}
}

// copyTxJsonToClipboard copies transaction JSON to clipboard
func copyTxJsonToClipboard(text string) tea.Cmd {
	return func() tea.Msg {
		err := clipboard.WriteAll(text)
		if err == nil {
			return txJsonCopiedMsg{}
		}
		return nil
	}
}

// clearClipboardMsg waits 2 seconds then sends a message to clear clipboard feedback
func clearClipboardMsg() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return struct{ clearClipboard bool }{true}
	})
}

// lookupENS performs reverse ENS lookup (address -> name)
func lookupENS(client *rpc.Client, address string) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return ensLookupResultMsg{address: address, ensName: "", err: fmt.Errorf("no RPC client"), debugInfo: ""}
		}

		// Perform ENS reverse lookup
		result := helpers.LookupENS(address, client.URL)
		return ensLookupResultMsg{
			address:   address,
			ensName:   result.Name,
			err:       result.Error,
			debugInfo: result.DebugInfo,
		}
	}
}

// resolveENS performs forward ENS resolution (name -> address)
func resolveENS(client *rpc.Client, ensName string) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return ensForwardResolveMsg{ensName: ensName, address: "", err: fmt.Errorf("no RPC client"), debugInfo: ""}
		}

		// Perform ENS forward resolution
		result := helpers.ResolveENS(ensName, client.URL)
		return ensForwardResolveMsg{
			ensName:   ensName,
			address:   result.Name, // Name field contains the resolved address
			err:       result.Error,
			debugInfo: result.DebugInfo,
		}
	}
}

// fetchUniswapQuote fetches a swap quote from Uniswap V2
func fetchUniswapQuote(client *rpc.Client, pairAddr, tokenInAddr common.Address, amountIn *big.Int) tea.Cmd {
	return func() tea.Msg {
		if client == nil || client.Client == nil {
			return uniswapQuoteMsg{nil, fmt.Errorf("no RPC client")}
		}

		quote, err := helpers.GetSwapQuote(client.Client, pairAddr, tokenInAddr, amountIn)
		return uniswapQuoteMsg{quote, err}
	}
}

// fetchReverseUniswapQuote calculates required input amount for desired output
func fetchReverseUniswapQuote(client *rpc.Client, pairAddr, tokenInAddr common.Address, amountOut *big.Int, fromTokenDecimals uint8) tea.Cmd {
	return func() tea.Msg {
		if client == nil || client.Client == nil {
			return uniswapQuoteMsg{nil, fmt.Errorf("no RPC client")}
		}

		ctx := context.Background()

		// Get the pair info
		pair, err := helpers.GetUniswapV2Pair(client.Client, pairAddr)
		if err != nil {
			return uniswapQuoteMsg{nil, err}
		}

		// Get reserves using same method as GetSwapQuote
		getReservesSelector := []byte{0x09, 0x02, 0xf1, 0xac}
		reservesMsg := ethereum.CallMsg{
			To:   &pairAddr,
			Data: getReservesSelector,
		}
		reservesData, err := client.Client.CallContract(ctx, reservesMsg, nil)
		if err != nil {
			return uniswapQuoteMsg{nil, fmt.Errorf("failed to get reserves: %w", err)}
		}

		// Parse reserves
		if len(reservesData) < 32 {
			return uniswapQuoteMsg{nil, fmt.Errorf("invalid reserves data length: %d", len(reservesData))}
		}

		reserve0 := new(big.Int).SetBytes(reservesData[0:32])
		reserve1 := big.NewInt(0)
		if len(reservesData) >= 64 {
			reserve1 = new(big.Int).SetBytes(reservesData[32:64])
		}

		// Determine which reserve is which based on token order
		var reserveIn, reserveOut *big.Int
		if tokenInAddr == pair.Token0 {
			reserveIn = reserve0
			reserveOut = reserve1
		} else {
			reserveIn = reserve1
			reserveOut = reserve0
		}

		// Calculate required input amount using reverse formula:
		// amountIn = (reserveIn * amountOut * 1000) / ((reserveOut - amountOut) * 997) + 1
		numerator := new(big.Int).Mul(reserveIn, amountOut)
		numerator = new(big.Int).Mul(numerator, big.NewInt(1000))

		denominator := new(big.Int).Sub(reserveOut, amountOut)
		denominator = new(big.Int).Mul(denominator, big.NewInt(997))

		if denominator.Sign() <= 0 {
			return uniswapQuoteMsg{nil, fmt.Errorf("insufficient liquidity for desired output amount")}
		}

		amountIn := new(big.Int).Div(numerator, denominator)
		amountIn = new(big.Int).Add(amountIn, big.NewInt(1)) // Add 1 for rounding

		// Calculate effective price
		effectivePrice := 0.0
		if amountIn.Sign() > 0 {
			amountInFloat := new(big.Float).SetInt(amountIn)
			amountOutFloat := new(big.Float).SetInt(amountOut)
			priceFloat := new(big.Float).Quo(amountOutFloat, amountInFloat)
			effectivePrice, _ = priceFloat.Float64()
		}

		// Calculate price impact
		priceImpact := 0.0
		if reserveIn.Sign() > 0 && reserveOut.Sign() > 0 {
			spotPrice := new(big.Float).Quo(new(big.Float).SetInt(reserveOut), new(big.Float).SetInt(reserveIn))
			spotPriceFloat, _ := spotPrice.Float64()

			if spotPriceFloat > 0 {
				priceImpact = ((spotPriceFloat - effectivePrice) / spotPriceFloat) * 100
			}
		}

		quote := &helpers.SwapQuote{
			AmountIn:       amountIn,
			AmountOut:      amountOut,
			Token0Reserve:  reserve0,
			Token1Reserve:  reserve1,
			PriceImpact:    priceImpact,
			EffectivePrice: effectivePrice,
		}

		return uniswapQuoteMsg{quote, nil}
	}
}

// fetchTerraNumberOfClaims fetches the total number of claims from Terra Nullius
func fetchTerraNumberOfClaims(client *rpc.Client) tea.Cmd {
	return func() tea.Msg {
		if client == nil || client.Client == nil {
			return terraNullClaimsCountMsg{nil, fmt.Errorf("no RPC client")}
		}
		count, err := helpers.GetTerraNumberOfClaims(client.Client)
		return terraNullClaimsCountMsg{count, err}
	}
}

// fetchTerraClaim fetches a specific claim by index from Terra Nullius
func fetchTerraClaim(client *rpc.Client, index *big.Int) tea.Cmd {
	return func() tea.Msg {
		if client == nil || client.Client == nil {
			return terraNullClaimQueryMsg{nil, fmt.Errorf("no RPC client")}
		}
		result, err := helpers.GetTerraClaim(client.Client, index)
		return terraNullClaimQueryMsg{result, err}
	}
}

// logLinkRegion describes the visual column range [startCol, endCol) of a single
// OSC 8 hyperlink within a log line and the URL it points to.
type logLinkRegion struct {
	startCol int
	endCol   int
	url      string
}

// parseOSC8Links walks a raw log buffer line and returns the visual column
// range for every OSC 8 hyperlink found in it. Column positions are computed
// using ansi.StringWidth so that ANSI SGR colour codes are treated as
// zero-width, matching what the terminal actually renders.
func parseOSC8Links(line string) []logLinkRegion {
	const osc8Prefix = "\x1b]8;;"
	var regions []logLinkRegion
	visualCol := 0  // accumulated visual width of text already consumed
	remaining := line

	for {
		idx := strings.Index(remaining, osc8Prefix)
		if idx < 0 {
			break
		}
		// Add visual width of plain text (and any other escape sequences) before this link.
		visualCol += ansi.StringWidth(remaining[:idx])
		after := remaining[idx+len(osc8Prefix):]

		// Read URL up to BEL.
		belIdx := strings.IndexByte(after, '\x07')
		if belIdx < 0 {
			break
		}
		url := after[:belIdx]
		afterBEL := after[belIdx+1:]

		if url == "" {
			// This is a reset sequence — skip it and continue scanning.
			remaining = afterBEL
			continue
		}

		// Find the matching reset (next OSC 8 sequence).
		resetIdx := strings.Index(afterBEL, osc8Prefix)
		if resetIdx < 0 {
			break
		}
		displayText := afterBEL[:resetIdx]
		displayWidth := ansi.StringWidth(displayText)

		regions = append(regions, logLinkRegion{
			startCol: visualCol,
			endCol:   visualCol + displayWidth,
			url:      url,
		})
		visualCol += displayWidth

		// Skip past the reset sequence (OSC 8 ;; BEL).
		afterDisplay := afterBEL[resetIdx+len(osc8Prefix):]
		resetBEL := strings.IndexByte(afterDisplay, '\x07')
		if resetBEL < 0 {
			break
		}
		remaining = afterDisplay[resetBEL+1:]
	}
	return regions
}

// urlAtCol returns the URL of whichever OSC 8 hyperlink occupies visual column
// col in line, or "" if col does not land on any hyperlink.
func urlAtCol(line string, col int) string {
	for _, r := range parseOSC8Links(line) {
		if col >= r.startCol && col < r.endCol {
			return r.url
		}
	}
	return ""
}

// loadRecentEvents fetches the most recent indexed events from the local store.
func loadRecentEvents(s *store.Store, limit int) tea.Cmd {
	return func() tea.Msg {
		events, err := s.RecentEvents(limit)
		if err != nil {
			return recentEventsMsg{err: err}
		}
		count, _ := s.Count()
		return recentEventsMsg{events: events, count: count}
	}
}

// waitForIndexedEvent blocks on the next event from the address indexer.
// Returns indexerStoppedMsg when the channel is closed.
func waitForIndexedEvent(idx *indexer.Indexer) tea.Cmd {
	return func() tea.Msg {
		event, ok := <-idx.Events()
		if !ok {
			return indexerStoppedMsg{}
		}
		return indexedEventMsg{event: event}
	}
}

// waitForV4PoolEvent blocks on the next Uniswap V4 PoolManager event from the indexer.
// Returns v4PoolIndexerStoppedMsg when the channel is closed.
func waitForV4PoolEvent(idx *indexer.Indexer) tea.Cmd {
	return func() tea.Msg {
		event, ok := <-idx.PoolEvents()
		if !ok {
			return v4PoolIndexerStoppedMsg{}
		}
		return v4PoolEventMsg{event: event}
	}
}

// waitForIndexerProgress blocks on the next backward-scan progress tick.
// Returns nil when the channel is closed (indexer stopped).
func waitForIndexerProgress(idx *indexer.Indexer) tea.Cmd {
	return func() tea.Msg {
		block, ok := <-idx.Progress()
		if !ok {
			return nil
		}
		return indexerProgressMsg{block: block}
	}
}

// openInBrowser opens url in the system default browser (macOS: open, Linux: xdg-open).
func openInBrowser(url string) tea.Cmd {
	return func() tea.Msg {
		_ = exec.Command("open", url).Start()
		return nil
	}
}

// fetchLiquidityPositions looks up all Uniswap V4 NFT positions held by ownerAddr.
func fetchLiquidityPositions(rpcURL string, ownerAddr common.Address) tea.Cmd {
	return func() tea.Msg {
		positions, nftCount, diags, err := helpers.GetLiquidityPositions(rpcURL, ownerAddr)
		return liquidityPositionsMsg{positions: positions, nftCount: nftCount, diagnostics: diags, err: err}
	}
}

// fetchPoolInfo calls getSlot0 and getLiquidity on the V4 StateView for poolIDHex.
func fetchPoolInfo(rpcURL, poolIDHex string) tea.Cmd {
	return func() tea.Msg {
		id := common.HexToHash(poolIDHex)
		info, err := helpers.FetchPoolInfo(rpcURL, id)
		return poolInfoResultMsg{poolID: poolIDHex, info: info, err: err}
	}
}

// fetchPoolKey looks up the Initialize event log for poolIDHex to get currency0/1, fee, etc.
func fetchPoolKey(rpcURL, poolIDHex string) tea.Cmd {
	return func() tea.Msg {
		id := common.HexToHash(poolIDHex)
		key, err := helpers.FetchPoolKey(rpcURL, id)
		return poolKeyResultMsg{poolID: poolIDHex, key: key, err: err}
	}
}

// waitForV4BlockScanLine blocks until the next line arrives from a V4BlockScanner.
// Returns v4BlockScanDoneMsg when the channel is closed (scan finished or stopped).
func waitForV4BlockScanLine(scanner *helpers.V4BlockScanner) tea.Cmd {
	return func() tea.Msg {
		line, ok := <-scanner.Lines()
		if !ok {
			return v4BlockScanDoneMsg{}
		}
		return v4BlockScanLineMsg{line: line}
	}
}

// loadV4PoolTableCmd queries all indexed V4 pools with aggregated stats and returns a v4PoolTableMsg.
func loadV4PoolTableCmd(s *store.Store) tea.Cmd {
	return func() tea.Msg {
		if s == nil {
			return v4PoolTableMsg{}
		}
		rows, err := s.V4PoolStats()
		if err != nil {
			return v4PoolTableMsg{}
		}
		return v4PoolTableMsg{rows: rows}
	}
}

// waitForPoolEventData blocks until the next structured V4PoolEvent arrives from the monitor.
// Returns poolMonitorEventMsg, or poolEventMonitorStoppedMsg when the channel is closed.
func waitForPoolEventData(monitor *helpers.PoolEventMonitor) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-monitor.Events()
		if !ok {
			return poolEventMonitorStoppedMsg{}
		}
		return poolMonitorEventMsg{event: ev}
	}
}

// waitForPoolEvent blocks until the next line arrives from the pool event monitor channel.
// Returns poolEventLineMsg when a line is received, or poolEventMonitorStoppedMsg when
// the channel is closed (monitor stopped or error).
func waitForPoolEvent(monitor *helpers.PoolEventMonitor) tea.Cmd {
	return func() tea.Msg {
		line, ok := <-monitor.Lines()
		if !ok {
			return poolEventMonitorStoppedMsg{}
		}
		return poolEventLineMsg{line: line}
	}
}

// packageTerraClaimTx packages a Terra Nullius claim transaction for QR display using EIP-4527 encoding.
func packageTerraClaimTx(fromAddr, message, rpcURL string) tea.Cmd {
	return func() tea.Msg {
		calldata := helpers.BuildTerraClaimCalldata(message)
		fromAddress := common.HexToAddress(fromAddr)
		toAddress := common.HexToAddress(helpers.TerraContractAddress)

		urStr, err := rpc.PackUnsignedTxEIP4527(fromAddress, toAddress, big.NewInt(0), 100000, calldata, rpcURL)
		if err != nil {
			return packageTransactionMsg{err: err}
		}

		summary := fmt.Sprintf("Terra Nullius claim: \"%s\"\nContract: %s\n\n%s",
			message, helpers.TerraContractAddress, urStr)
		return packageTransactionMsg{txDisplay: summary, qrData: urStr, format: "EIP-4527", err: nil}
	}
}

// -------------------- MODEL HELPER METHODS --------------------
// These methods help with state management and command generation

// logIndexedEvent logs a single IndexedEvent in the same detailed format used by the test output.
func (m *model) logIndexedEvent(ev indexer.IndexedEvent) {
	divisor := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(ev.Decimals)), nil))
	humanAmt := new(big.Float).Quo(new(big.Float).SetInt(ev.Value), divisor)
	m.addLog("info", fmt.Sprintf("  Token   : %s (%s)", ev.Symbol, helpers.HyperAddr(ev.Token)))
	m.addLog("info", fmt.Sprintf("  Block   : %d", ev.Block))
	m.addLog("info", fmt.Sprintf("  TxHash  : %s", helpers.HyperTxHash(ev.TxHash)))
	m.addLog("info", fmt.Sprintf("  LogIndex: %d", ev.LogIndex))
	m.addLog("info", fmt.Sprintf("  From    : %s", helpers.HyperAddr(ev.From)))
	m.addLog("info", fmt.Sprintf("  To      : %s", helpers.HyperAddr(ev.To)))
	m.addLog("info", fmt.Sprintf("  Value   : %s raw  (%s %s)", ev.Value.String(), fmt.Sprintf("%.6f", humanAmt), ev.Symbol))
	m.addLog("info", fmt.Sprintf("  Decimals: %d", ev.Decimals))
	m.addLog("info", "  ─────────────────────────────────────────────────────────")
}

// logV4PoolEvent logs a single V4PoolEvent in a structured, human-readable format.
func (m *model) logV4PoolEvent(ev indexer.V4PoolEvent) {
	bigStr := func(x *big.Int) string {
		if x == nil {
			return "0"
		}
		return x.String()
	}
	shortPool := func(h common.Hash) string {
		s := h.Hex()
		return helpers.FadeString(s[:10]+"…"+s[len(s)-6:], "#7EE787", "#82CFFD")
	}
	sep := "  ─────────────────────────────────────────────────────────"

	switch ev.Kind {
	case indexer.V4KindSwap:
		dir := "→"
		if ev.Amount0 != nil && ev.Amount0.Sign() > 0 {
			dir = "←"
		}
		m.addLog("info", fmt.Sprintf("[V4-SWAP] %s  Pool: %s", dir, shortPool(ev.PoolID)))
		m.addLog("info", fmt.Sprintf("  Sender    : %s", helpers.HyperAddr(ev.Sender)))
		m.addLog("info", fmt.Sprintf("  Amount0   : %s", bigStr(ev.Amount0)))
		m.addLog("info", fmt.Sprintf("  Amount1   : %s", bigStr(ev.Amount1)))
		m.addLog("info", fmt.Sprintf("  Tick      : %s", bigStr(ev.Tick)))
		m.addLog("info", fmt.Sprintf("  Block     : %d", ev.Block))
		m.addLog("info", fmt.Sprintf("  TxHash    : %s", helpers.HyperTxHash(ev.TxHash)))
		m.addLog("info", sep)

	case indexer.V4KindModifyLiquidity:
		sign := "+"
		if ev.LiquidityDelta != nil && ev.LiquidityDelta.Sign() < 0 {
			sign = "-"
		}
		m.addLog("info", fmt.Sprintf("[V4-LIQ] %sΔ  Pool: %s", sign, shortPool(ev.PoolID)))
		m.addLog("info", fmt.Sprintf("  Sender    : %s", helpers.HyperAddr(ev.Sender)))
		m.addLog("info", fmt.Sprintf("  ΔLiquidity: %s", bigStr(ev.LiquidityDelta)))
		m.addLog("info", fmt.Sprintf("  Ticks     : [%s, %s]", bigStr(ev.TickLower), bigStr(ev.TickUpper)))
		m.addLog("info", fmt.Sprintf("  Block     : %d", ev.Block))
		m.addLog("info", fmt.Sprintf("  TxHash    : %s", helpers.HyperTxHash(ev.TxHash)))
		m.addLog("info", sep)

	case indexer.V4KindDonate:
		m.addLog("info", fmt.Sprintf("[V4-DONATE]  Pool: %s", shortPool(ev.PoolID)))
		m.addLog("info", fmt.Sprintf("  Sender  : %s", helpers.HyperAddr(ev.Sender)))
		m.addLog("info", fmt.Sprintf("  Amount0 : %s", bigStr(ev.Amount0)))
		m.addLog("info", fmt.Sprintf("  Amount1 : %s", bigStr(ev.Amount1)))
		m.addLog("info", fmt.Sprintf("  Block   : %d", ev.Block))
		m.addLog("info", fmt.Sprintf("  TxHash  : %s", helpers.HyperTxHash(ev.TxHash)))
		m.addLog("info", sep)

	case indexer.V4KindTransfer:
		m.addLog("info", fmt.Sprintf("[V4-TRANSFER]  TokenID: %s", bigStr(ev.TokenID)))
		m.addLog("info", fmt.Sprintf("  From    : %s", helpers.HyperAddr(ev.From)))
		m.addLog("info", fmt.Sprintf("  To      : %s", helpers.HyperAddr(ev.To)))
		m.addLog("info", fmt.Sprintf("  Amount  : %s", bigStr(ev.Amount0)))
		m.addLog("info", fmt.Sprintf("  Block   : %d", ev.Block))
		m.addLog("info", fmt.Sprintf("  TxHash  : %s", helpers.HyperTxHash(ev.TxHash)))
		m.addLog("info", sep)
	}
}

// addLog adds a log entry with timestamp and type
func (m *model) addLog(logType, message string) {
	if !m.logEnabled || !m.logReady || m.logger == nil {
		return
	}

	// Use the logger to write messages
	switch logType {
	case "info":
		m.logger.Info(message)
	case "success":
		m.logger.Info("✓", "msg", message)
	case "error":
		m.logger.Error(message)
	case "warning":
		m.logger.Warn(message)
	case "debug":
		m.logger.Debug(message)
	default:
		m.logger.Print(message)
	}

	// Update viewport content
	m.updateLogViewport()
}

// loadSelectedWalletDetails loads details for the currently selected wallet if split view is enabled
func (m *model) loadSelectedWalletDetails() tea.Cmd {
	if !m.detailsInWallets || len(m.accounts) == 0 {
		return nil
	}

	addr := m.accounts[m.selectedWallet].Address
	// Check if we have cached details
	cachedDetails, hasCached := m.detailsCache[strings.ToLower(addr)]
	if hasCached {
		m.details = cachedDetails
		m.loading = false
		return nil
	}

	// Load fresh details
	m.loading = true
	m.details = config.WalletDetails{Address: addr}
	ethAddr := common.HexToAddress(addr)
	return loadDetails(m.ethClient, ethAddr, m.tokenWatch)
}

// colorizeLogContent applies color coding to log messages
func colorizeLogContent(content string) string {
	if content == "" {
		return content
	}

	var result strings.Builder
	lines := strings.Split(content, "\n")

	// Define color styles for log levels
	errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#F25D94")).Bold(true)
	infoStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#7EE787")).Bold(true)
	debugStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#79C0FF")).Bold(true)
	checkStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#7EE787"))

	for i, line := range lines {
		if i > 0 {
			result.WriteString("\n")
		}

		// Color check marks
		coloredLine := strings.ReplaceAll(line, "✓", checkStyle.Render("✓"))

		// Color log level keywords (they appear after timestamp, like "15:04:05 INFO message")
		if strings.Contains(coloredLine, " ERROR") {
			coloredLine = strings.Replace(coloredLine, " ERROR", " "+errorStyle.Render("ERROR"), 1)
		} else if strings.Contains(coloredLine, " INFO") {
			coloredLine = strings.Replace(coloredLine, " INFO", " "+infoStyle.Render("INFO"), 1)
		} else if strings.Contains(coloredLine, " DEBUG") {
			coloredLine = strings.Replace(coloredLine, " DEBUG", " "+debugStyle.Render("DEBUG"), 1)
		}

		result.WriteString(coloredLine)
	}

	return result.String()
}

// maxLogBytes is the maximum size of the in-memory log buffer (~2 MB).
const maxLogBytes = 2 * 1024 * 1024

// It preserves the current scroll position so that manual scrolls are not
// overridden; it only jumps to the bottom if the viewport was already there.
func (m *model) updateLogViewport() {
	if !m.logReady || m.logBuffer == nil {
		return
	}

	// Remember whether we were at the bottom before refreshing content.
	atBottom := m.logViewport.AtBottom()

	content := m.logBuffer.String()

	// Trim the oldest entries if the buffer has grown beyond the cap.
	if len(content) > maxLogBytes {
		trimmed := content[len(content)-maxLogBytes:]
		// Advance to the start of the next complete line.
		if idx := strings.Index(trimmed, "\n"); idx >= 0 {
			trimmed = trimmed[idx+1:]
		}
		m.logBuffer.Reset()
		m.logBuffer.WriteString(trimmed)
		content = trimmed
	}

	content = colorizeLogContent(content)
	m.logViewport.SetContent(content)

	// Only follow new output when the user hasn't scrolled up.
	if atBottom {
		m.logViewport.GotoBottom()
	}
}

// textInputActive returns true if any text input is currently active
func (m model) textInputActive() bool {
	if m.adding {
		return true
	}
	if m.showSendForm && m.sendForm != nil {
		return true
	}
	if m.nicknaming && m.form != nil {
		return true
	}
	if (m.settingsMode == "add" || m.settingsMode == "edit") && m.form != nil {
		return true
	}
if m.activeDialog == dialogTerraClaim {
		return true
	}
	return false
}

// buildTokenList builds a list of available tokens from wallet details for Uniswap
func (m model) buildTokenList() []uniswap.TokenOption {
	var tokens []uniswap.TokenOption

	// Build a symbol→balance/decimals lookup from loaded wallet tokens
	type heldToken struct {
		balance  *big.Int
		decimals uint8
	}
	held := make(map[string]heldToken, len(m.details.Tokens))
	for _, token := range m.details.Tokens {
		held[token.Symbol] = heldToken{balance: token.Balance, decimals: token.Decimals}
	}

	// Add ETH first (address left as zero; swaps use WETH address in calldata path)
	tokens = append(tokens, uniswap.TokenOption{
		Symbol:   "ETH",
		Balance:  m.details.EthWei,
		Decimals: 18,
		IsETH:    true,
	})

	// Add all tokens from the watchlist; fill in actual balance where the wallet holds them
	for _, wt := range m.tokenWatch {
		opt := uniswap.TokenOption{
			Symbol:   wt.Symbol,
			Decimals: wt.Decimals,
			IsETH:    false,
			Address:  wt.Address,
		}
		if h, ok := held[wt.Symbol]; ok {
			opt.Balance = h.balance
			opt.Decimals = h.decimals
		}
		tokens = append(tokens, opt)
	}

	return tokens
}

// maybeRequestUniswapQuote triggers a swap quote fetch if conditions are met (forward quote: calculate output from input)
func (m *model) maybeRequestUniswapQuote() tea.Cmd {
	// Check if we have valid inputs
	if m.uniswapFromAmount == "" || m.uniswapFromAmount == "0" {
		// Clear previous quote state when amount is cleared
		m.uniswapToAmount = ""
		m.uniswapQuote = nil
		m.uniswapQuoteError = ""
		m.uniswapPriceImpactWarn = ""
		m.lastQuoteFromAmount = ""
		return nil
	}

	tokens := m.buildTokenList()
	if m.uniswapFromTokenIdx < 0 || m.uniswapFromTokenIdx >= len(tokens) {
		return nil
	}
	if m.uniswapToTokenIdx < 0 || m.uniswapToTokenIdx >= len(tokens) {
		return nil
	}

	fromToken := tokens[m.uniswapFromTokenIdx]
	toToken := tokens[m.uniswapToTokenIdx]

	// Can't swap same token
	if fromToken.Symbol == toToken.Symbol {
		return nil
	}

	// Check if anything has changed since last quote
	if m.lastQuoteFromAmount == m.uniswapFromAmount &&
		m.lastQuoteFromTokenIdx == m.uniswapFromTokenIdx &&
		m.lastQuoteToTokenIdx == m.uniswapToTokenIdx &&
		m.uniswapQuote != nil &&
		m.uniswapToAmount != "" {
		// Nothing changed and we already have a quote, no need to fetch again
		return nil
	}

	// Parse amount
	amountFloat := new(big.Float)
	_, ok := amountFloat.SetString(m.uniswapFromAmount)
	if !ok {
		return nil
	}

	// Convert to token's base unit (e.g., wei for ETH, smallest unit for tokens)
	multiplier := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(fromToken.Decimals)), nil))
	amountInTokenUnits := new(big.Float).Mul(amountFloat, multiplier)
	amountIn, _ := amountInTokenUnits.Int(nil)

	if amountIn == nil || amountIn.Sign() <= 0 {
		return nil
	}

	// Determine pair address and token addresses
	// For now, only support USDC <-> WETH swaps
	var pairAddr, tokenInAddr common.Address

	// Map token symbols to addresses
	if (fromToken.Symbol == "USDC" && toToken.Symbol == "ETH") || (fromToken.Symbol == "ETH" && toToken.Symbol == "USDC") {
		pairAddr = helpers.USDCWETHPairAddress
		if fromToken.Symbol == "USDC" {
			tokenInAddr = helpers.USDCAddress
		} else {
			tokenInAddr = helpers.WETHAddress
		}
	} else {
		// Unsupported pair
		m.addLog("warn", fmt.Sprintf("Swap pair %s/%s not supported yet", fromToken.Symbol, toToken.Symbol))
		return nil
	}

	// Update tracking state before fetching
	m.lastQuoteFromAmount = m.uniswapFromAmount
	m.lastQuoteFromTokenIdx = m.uniswapFromTokenIdx
	m.lastQuoteToTokenIdx = m.uniswapToTokenIdx

	// Clear previous quote state when fetching new quote
	m.uniswapToAmount = ""
	m.uniswapQuote = nil
	m.uniswapQuoteError = ""
	m.uniswapPriceImpactWarn = ""

	m.uniswapEstimating = true
	return fetchUniswapQuote(m.ethClient, pairAddr, tokenInAddr, amountIn)
}

// maybeRequestReverseUniswapQuote triggers a reverse swap quote fetch (calculate From from To amount)
func (m *model) maybeRequestReverseUniswapQuote() tea.Cmd {
	// Check if we have valid inputs
	if m.uniswapToAmount == "" || m.uniswapToAmount == "0" {
		// Clear previous quote state when amount is cleared
		m.uniswapFromAmount = ""
		m.uniswapQuote = nil
		m.uniswapQuoteError = ""
		m.uniswapPriceImpactWarn = ""
		return nil
	}

	tokens := m.buildTokenList()
	if m.uniswapFromTokenIdx < 0 || m.uniswapFromTokenIdx >= len(tokens) {
		return nil
	}
	if m.uniswapToTokenIdx < 0 || m.uniswapToTokenIdx >= len(tokens) {
		return nil
	}

	fromToken := tokens[m.uniswapFromTokenIdx]
	toToken := tokens[m.uniswapToTokenIdx]

	// Can't swap same token
	if fromToken.Symbol == toToken.Symbol {
		return nil
	}

	// Check if anything has changed since last reverse quote
	if m.lastQuoteToAmount == m.uniswapToAmount &&
		m.lastQuoteFromTokenIdx == m.uniswapFromTokenIdx &&
		m.lastQuoteToTokenIdx == m.uniswapToTokenIdx &&
		m.uniswapQuote != nil &&
		m.uniswapFromAmount != "" {
		// Nothing changed and we already have a quote, no need to fetch again
		return nil
	}

	// Parse desired output amount
	amountOutFloat := new(big.Float)
	_, ok := amountOutFloat.SetString(m.uniswapToAmount)
	if !ok {
		return nil
	}

	// Convert to token's base unit
	multiplier := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(toToken.Decimals)), nil))
	amountOutTokenUnits := new(big.Float).Mul(amountOutFloat, multiplier)
	amountOut, _ := amountOutTokenUnits.Int(nil)

	if amountOut == nil || amountOut.Sign() <= 0 {
		return nil
	}

	// Determine pair address and token addresses
	var pairAddr, tokenInAddr common.Address

	if (fromToken.Symbol == "USDC" && toToken.Symbol == "ETH") || (fromToken.Symbol == "ETH" && toToken.Symbol == "USDC") {
		pairAddr = helpers.USDCWETHPairAddress
		if fromToken.Symbol == "USDC" {
			tokenInAddr = helpers.USDCAddress
		} else {
			tokenInAddr = helpers.WETHAddress
		}
	} else {
		// Unsupported pair
		m.addLog("warn", fmt.Sprintf("Swap pair %s/%s not supported yet", fromToken.Symbol, toToken.Symbol))
		return nil
	}

	// Update tracking state before fetching
	m.lastQuoteToAmount = m.uniswapToAmount
	m.lastQuoteFromTokenIdx = m.uniswapFromTokenIdx
	m.lastQuoteToTokenIdx = m.uniswapToTokenIdx

	// Clear previous from amount and quote state
	m.uniswapFromAmount = ""
	m.uniswapQuote = nil
	m.uniswapQuoteError = ""
	m.uniswapPriceImpactWarn = ""
	m.uniswapEstimating = true

	m.addLog("info", fmt.Sprintf("Calculating required input for %s %s", m.uniswapToAmount, toToken.Symbol))

	return fetchReverseUniswapQuote(m.ethClient, pairAddr, tokenInAddr, amountOut, fromToken.Decimals)
}

// navigateTo sets the active page and returns any initial Cmd required
// for that page (e.g. data fetches, state resets). Callers should use
//   return m, m.navigateTo(config.PageXxx)
// instead of setting m.activePage inline.
func (m *model) navigateTo(page config.Page) tea.Cmd {
	m.activePage = page
	switch page {
	case config.PageWallets:
		return m.loadSelectedWalletDetails()
	case config.PageSettings:
		m.settingsMode = "list"
	case config.PageUniswap:
		m.uniswapFromTokenIdx = 0
		m.uniswapToTokenIdx = 1
		m.uniswapFromAmount = ""
		m.uniswapToAmount = ""
		m.uniswapFocusedField = 0
		m.uniswapShowingSelector = false
		m.uniswapSelectorFor = 0
		m.uniswapSelectorIdx = 0
		m.uniswapEstimating = false
		m.uniswapQuote = nil
		m.uniswapQuoteError = ""
		m.uniswapPriceImpactWarn = ""
		m.lastQuoteFromAmount = ""
		m.lastQuoteFromTokenIdx = -1
		m.lastQuoteToTokenIdx = -1
	case config.PageTerraNullius:
		m.terraNullFocusedField = 1
		m.terraNullClaimsCount = ""
		m.terraNullClaimsLoading = true
		m.terraNullClaimInput = "0"
		m.terraNullClaimResult = nil
		m.terraNullClaimQuerying = false
		m.terraNullClaimResultErr = ""
		m.activeDialog = dialogNone
		m.addLog("info", "Terra Nullius: loading number of claims…")
		return fetchTerraNumberOfClaims(m.ethClient)
	}
	return nil
}

// -------------------- WEBCAM COMMANDS --------------------

// openWebcamCmd opens the camera and starts streaming. Returns webcamReadyMsg or webcamErrMsg.
func openWebcamCmd() tea.Msg {
	cam, err := capture.New("")
	if err != nil {
		return webcamErrMsg{err}
	}
	if err := cam.Start(); err != nil {
		cam.Close()
		return webcamErrMsg{err}
	}
	return webcamReadyMsg{cam: cam, ch: cam.Frames()}
}

// waitForWebcamFrame blocks until the next camera frame arrives, then decodes any QR code in it.
func waitForWebcamFrame(ch <-chan image.Image) tea.Cmd {
	return func() tea.Msg {
		img, ok := <-ch
		if !ok {
			return webcamErrMsg{fmt.Errorf("webcam stream closed")}
		}
		return webcamFrameMsg{img: img, qrText: decodeQR(img)}
	}
}

// decodeQR attempts to extract a QR code string from img. Returns "" on failure.
func decodeQR(img image.Image) string {
	bmp, err := gozxing.NewBinaryBitmapFromImage(img)
	if err != nil {
		return ""
	}
	result, err := gozxingqr.NewQRCodeReader().Decode(bmp, nil)
	if err != nil {
		return ""
	}
	return result.String()
}
