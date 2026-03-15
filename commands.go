package main

import (
	"context"
	"fmt"
	"math/big"
	"os/exec"
	"strings"
	"time"

	"charm-wallet-tui/config"
	"charm-wallet-tui/helpers"
	"charm-wallet-tui/rpc"
	"charm-wallet-tui/views/uniswap"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
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

// packageTransaction packages an ETH transfer transaction for QR display
func packageTransaction(fromAddr, toAddr string, ethAmount string, rpcURL string) tea.Cmd {
	return func() tea.Msg {
		// Convert ETH amount to Wei
		amountFloat := new(big.Float)
		amountFloat.SetString(ethAmount)
		weiFloat := new(big.Float).Mul(amountFloat, big.NewFloat(1e18))
		amountWei, _ := weiFloat.Int(nil)

		// Call RPC package function using EIP-681 format
		pkg, err := rpc.PackageTransaction(
			common.HexToAddress(fromAddr),
			common.HexToAddress(toAddr),
			amountWei,
			rpcURL,
		)

		return packageTransactionMsg{txDisplay: pkg.EIP681, qrData: pkg.EIP681, format: "EIP-681", err: err}
	}
}

// packageSwapTransaction packages a Uniswap swap transaction for QR display
func packageSwapTransaction(fromAddr string, fromToken, toToken uniswap.TokenOption, amountIn string, amountOutMin *big.Int, rpcURL string) tea.Cmd {
	return func() tea.Msg {
		// For Uniswap V2, we need to call the router contract
		// Router address on mainnet: 0x7a250d5630B4cF539739dF2C5dAcb4c659F2488D
		routerAddr := "0x7a250d5630B4cF539739dF2C5dAcb4c659F2488D"

		// Convert amount to token's base unit
		amountFloat := new(big.Float)
		amountFloat.SetString(amountIn)
		multiplier := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(fromToken.Decimals)), nil))
		amountInUnits := new(big.Float).Mul(amountFloat, multiplier)
		amountInBig, _ := amountInUnits.Int(nil)

		// Build transaction description
		var txJSON string
		var eip681 string

		if fromToken.Symbol == "ETH" {
			// ETH -> Token swap: swapExactETHForTokens
			txJSON = fmt.Sprintf(`{
  "from": "%s",
  "to": "%s",
  "value": "0x%s",
  "data": "SWAP: %s %s -> %s (min %s)",
  "note": "Uniswap V2 Swap: %s %s to %s %s"
}`,
				fromAddr,
				routerAddr,
				amountInBig.Text(16),
				amountIn, fromToken.Symbol,
				toToken.Symbol,
				new(big.Float).Quo(new(big.Float).SetInt(amountOutMin), new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(toToken.Decimals)), nil))).Text('f', 6),
				amountIn, fromToken.Symbol,
				new(big.Float).Quo(new(big.Float).SetInt(amountOutMin), new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(toToken.Decimals)), nil))).Text('f', 6),
				toToken.Symbol,
			)
			eip681 = fmt.Sprintf("%s?value=%s", routerAddr, amountInBig.String())
		} else if toToken.Symbol == "ETH" {
			// Token -> ETH swap: swapExactTokensForETH
			txJSON = fmt.Sprintf(`{
  "from": "%s",
  "to": "%s",
  "value": "0x0",
  "data": "SWAP: %s %s -> %s (min %s)",
  "note": "Uniswap V2 Swap: %s %s to %s %s. IMPORTANT: Approve token first!"
}`,
				fromAddr,
				routerAddr,
				amountIn, fromToken.Symbol,
				toToken.Symbol,
				new(big.Float).Quo(new(big.Float).SetInt(amountOutMin), new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(toToken.Decimals)), nil))).Text('f', 6),
				amountIn, fromToken.Symbol,
				new(big.Float).Quo(new(big.Float).SetInt(amountOutMin), new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(toToken.Decimals)), nil))).Text('f', 6),
				toToken.Symbol,
			)
			eip681 = routerAddr
		} else {
			// Token -> Token swap: swapExactTokensForTokens
			txJSON = fmt.Sprintf(`{
  "from": "%s",
  "to": "%s",
  "value": "0x0",
  "data": "SWAP: %s %s -> %s (min %s)",
  "note": "Uniswap V2 Swap: %s %s to %s %s. IMPORTANT: Approve token first!"
}`,
				fromAddr,
				routerAddr,
				amountIn, fromToken.Symbol,
				toToken.Symbol,
				new(big.Float).Quo(new(big.Float).SetInt(amountOutMin), new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(toToken.Decimals)), nil))).Text('f', 6),
				amountIn, fromToken.Symbol,
				new(big.Float).Quo(new(big.Float).SetInt(amountOutMin), new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(toToken.Decimals)), nil))).Text('f', 6),
				toToken.Symbol,
			)
			eip681 = routerAddr
		}

		return packageTransactionMsg{txDisplay: txJSON, qrData: eip681, format: "EIP-4527", err: nil}
	}
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

// openInBrowser opens url in the system default browser (macOS: open, Linux: xdg-open).
func openInBrowser(url string) tea.Cmd {
	return func() tea.Msg {
		_ = exec.Command("open", url).Start()
		return nil
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

// packageTerraClaimTx packages a Terra Nullius claim transaction for QR display
func packageTerraClaimTx(fromAddr, message string) tea.Cmd {
	return func() tea.Msg {
		calldata := helpers.BuildTerraClaimCalldata(message)

		calldataHex := "0x"
		for _, b := range calldata {
			calldataHex += fmt.Sprintf("%02x", b)
		}

		txJSON := fmt.Sprintf(`{
  "from": "%s",
  "to": "%s",
  "value": "0x0",
  "data": "%s",
  "note": "Terra Nullius claim: %s"
}`,
			fromAddr,
			helpers.TerraContractAddress,
			calldataHex,
			message,
		)

		return packageTransactionMsg{
			txDisplay: txJSON,
			qrData:    helpers.TerraContractAddress,
			format:    "EIP-4527",
			err:       nil,
		}
	}
}

// -------------------- MODEL HELPER METHODS --------------------
// These methods help with state management and command generation

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

// updateLogViewport refreshes the viewport content with log output
func (m *model) updateLogViewport() {
	if !m.logReady || m.logBuffer == nil {
		return
	}

	// Get content from log buffer and apply color coding
	content := m.logBuffer.String()
	content = colorizeLogContent(content)
	m.logViewport.SetContent(content)
	// Scroll to bottom to show latest entries
	m.logViewport.GotoBottom()
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
if m.terraNullShowClaimForm {
		return true
	}
	return false
}

// buildTokenList builds a list of available tokens from wallet details for Uniswap
func (m model) buildTokenList() []uniswap.TokenOption {
	var tokens []uniswap.TokenOption

	// Add ETH first
	tokens = append(tokens, uniswap.TokenOption{
		Symbol:   "ETH",
		Balance:  m.details.EthWei,
		Decimals: 18,
		IsETH:    true,
	})

	// Add tokens from watchlist that have balances
	for _, token := range m.details.Tokens {
		tokens = append(tokens, uniswap.TokenOption{
			Symbol:   token.Symbol,
			Balance:  token.Balance,
			Decimals: token.Decimals,
			IsETH:    false,
		})
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
