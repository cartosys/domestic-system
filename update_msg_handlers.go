package main

import (
	"fmt"
	"math/big"
	"strings"
	"time"

	"charm-wallet-tui/anim"
	"charm-wallet-tui/config"
	"charm-wallet-tui/helpers"
	"charm-wallet-tui/indexer"
	"charm-wallet-tui/rpc"
	"charm-wallet-tui/styles"
	"charm-wallet-tui/views/txqr"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/charmbracelet/x/ansi"
	"github.com/ethereum/go-ethereum/common"
)

func (m *model) handleLogInit() (tea.Model, tea.Cmd) {
	if !m.logEnabled {
		return m, nil
	}
	m.initLogger()
	return m, nil
}

func (m *model) handleRPCConnected(msg rpcConnectedMsg) (tea.Model, tea.Cmd) {
	m.rpcConnecting = false
	if msg.err != nil {
		m.ethClient = nil
		m.rpcConnected = false
		m.logError(fmt.Sprintf("RPC connection failed: `%s`", msg.err.Error()))
	} else {
		m.ethClient = msg.client
		m.rpcConnected = true
		m.pairCache = make(map[string]pairCacheEntry) // chain may have changed
		m.logSuccess(fmt.Sprintf("RPC connected to `%s`", msg.client.URL))
		if m.activePage == config.PageWallets && m.detailsInWallets && len(m.accounts) > 0 {
			return m, m.loadSelectedWalletDetails()
		}
	}
	return m, nil
}

func (m *model) handleWindowSize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.w, m.h = msg.Width, msg.Height
	m.contentW = helpers.Max(0, m.w-2)
	if m.logEnabled {
		m.logViewport.Width = max(0, msg.Width-6)
		if m.logReady {
			m.updateLogViewport()
		}
	}
	m.v4EventsViewport.Width = max(0, msg.Width-8)
	m.poolListViewport.Width = max(0, msg.Width-8)
	m.syncPoolViewportHeights()
	m.refreshPoolViewports()
	m.tokenListViewport.Width = max(0, msg.Width-8)
	m.txQRViewport.Width = max(0, msg.Width-10)
	if m.activeDialog == dialogTxResult && m.txResultHex != "" {
		// The JSON/RLP table is sized to the viewport width; rebuild it.
		yOffset := m.txQRViewport.YOffset
		m.setTxViewportContent()
		m.txQRViewport.SetYOffset(yOffset)
	}
	return m, nil
}

func (m *model) handleSpinnerTick(msg anim.StepMsg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	cmds = append(cmds, m.spin.Animate(msg))
	if m.logEnabled && !m.logReady {
		cmds = append(cmds, m.logSpinner.Animate(msg))
	}
	return m, tea.Batch(cmds...)
}

func (m *model) handleDetailsLoaded(msg detailsLoadedMsg) (tea.Model, tea.Cmd) {
	m.loading = false
	m.details = msg.d
	if m.details.Address != "" {
		m.detailsCache[strings.ToLower(m.details.Address)] = m.details
	}
	if msg.err != nil && m.details.ErrMessage == "" {
		m.details.ErrMessage = "Failed to load wallet details."
		m.logError(fmt.Sprintf("Failed to load details for `%s`", helpers.ShortenAddr(m.details.Address)))
	} else if m.details.ErrMessage != "" {
		m.logError(fmt.Sprintf("Wallet `%s`: %s", helpers.ShortenAddr(m.details.Address), m.details.ErrMessage))
	} else {
		m.logSuccess(fmt.Sprintf("Loaded details for `%s` - ETH: %s", helpers.ShortenAddr(m.details.Address), helpers.FormatETH(m.details.EthWei)))
	}
	return m, nil
}

func (m *model) handlePackageTransaction(msg packageTransactionMsg) (tea.Model, tea.Cmd) {
	m.txResultPackaging = false
	if msg.err != nil {
		m.txResultError = msg.err.Error()
		m.logError("Transaction packaging failed: " + msg.err.Error())
		return m, nil
	}
	m.txResultFormat = msg.format

	renderFrames := func(urData string) []string {
		f, err := txqr.RenderAnimated(urData, 50)
		if err != nil || len(f) == 0 {
			return []string{txqr.Render(urData)}
		}
		return f
	}

	if msg.approveQRData != "" {
		// Two-step flow: approve (nonce N) then swap (nonce N+1).
		m.txApproveQRFrames = renderFrames(msg.approveQRData)
		m.txApproveJSON = msg.approveJSON
		m.txSwapQRFrames = renderFrames(msg.qrData)
		m.txSwapJSON = msg.txJSON
		m.txSwapSummary = msg.txDisplay
		m.txSwapStep = false // start on step 1 (approve)
		m.txQRFrames = m.txApproveQRFrames
		m.txQRFrameIdx = 0
		m.txResultHex = msg.approveJSON // Ctrl+C copies the active step's JSON
		m.txResultEIP681 = msg.approveQRData
		m.setTxViewportContent()
		m.logSuccess(fmt.Sprintf("Packaged approve + swap (%s) — sign Step 1 first", msg.format))
	} else {
		// Single-step flow.
		m.txApproveQRFrames = nil
		m.txApproveJSON = ""
		m.txSwapQRFrames = nil
		m.txSwapJSON = ""
		m.txSwapSummary = msg.txDisplay
		m.txSwapStep = false
		m.txQRFrames = renderFrames(msg.qrData)
		m.txQRFrameIdx = 0
		m.txResultHex = msg.txJSON
		m.txResultEIP681 = msg.qrData
		m.setTxViewportContent()
		m.logSuccess(fmt.Sprintf("Transaction packaged (%s, %d QR frame(s))", msg.format, len(m.txQRFrames)))
	}
	return m, animateQRTick()
}

// setTxViewportContent rebuilds the scrollable text content for the tx result
// dialog from current model state.
func (m *model) setTxViewportContent() {
	muteStyle := lipgloss.NewStyle().Foreground(styles.CMuted)
	warnStyle := lipgloss.NewStyle().Foreground(styles.CWarn).Bold(true)
	stepStyle := lipgloss.NewStyle().Foreground(styles.CAccent2).Bold(true)

	var prefix, tbl, suffix string
	if m.txApproveQRFrames != nil {
		if !m.txSwapStep {
			prefix = stepStyle.Render("Step 1 of 2: Approve token spend") + "\n" +
				warnStyle.Render("Sign and broadcast this transaction before the swap.") + "\n\n"
			tbl = m.renderTxDataTable("Approve transaction (JSON):", m.txApproveJSON)
			suffix = "\n\n" +
				muteStyle.Render("Scan the animated QR • Ctrl+C to copy JSON • Tab → Step 2 • ESC to close")
		} else {
			prefix = stepStyle.Render("Step 2 of 2: Swap") + "\n" +
				muteStyle.Render("Sign after the approve (Step 1) has confirmed on-chain.") + "\n\n" +
				m.txSwapSummary + "\n\n"
			tbl = m.renderTxDataTable("Swap transaction (JSON):", m.txSwapJSON)
			suffix = "\n\n" +
				muteStyle.Render("Scan the animated QR • Ctrl+C to copy JSON • Tab → Step 1 • ESC to close")
		}
	} else {
		prefix = m.txSwapSummary + "\n\n"
		tbl = m.renderTxDataTable("Transaction data (JSON):", m.txResultHex)
		suffix = "\n\n" +
			muteStyle.Render("Scan the animated QR with your air-gapped wallet to sign") + "\n" +
			muteStyle.Render("↑/↓ or j/k to scroll • Ctrl+C to copy • Enter to scan response • ESC to close")
	}
	if m.txTableLine >= 0 {
		m.txTableLine += strings.Count(prefix, "\n")
	}
	m.txQRViewport.Height = m.txQRViewportHeight()
	m.txQRViewport.SetContent(prefix + tbl + suffix)
	m.txQRViewport.GotoTop()
}

// renderTxDataTable renders a packaged tx as a single-row, two-cell table:
// its JSON on the left and its unsigned RLP hex (with a 📋 copy icon) on the
// right, each titled with a 📋 copy icon (clicking anywhere in a cell copies
// it). It sets m.txResultRLP to the hex, and m.txTableLine/Height/ColX to the
// table's extent within the returned string, for click hit-testing in
// renderTxResultContent.
func (m *model) renderTxDataTable(jsonLabel, txJSON string) string {
	labelStyle := lipgloss.NewStyle().Foreground(styles.CSuccess)
	warnStyle := lipgloss.NewStyle().Foreground(styles.CWarn).Bold(true)

	rlpHex, rlpErr := rpc.UnsignedTxRLPHex(txJSON)
	m.txResultRLP = rlpHex

	// Inner width left for cell text after 3 vertical borders + (0,1) padding on both cells.
	avail := max(2, m.txQRViewport.Width-7)
	// JSON keeps its natural width when it fits; the hex (which can break
	// anywhere) takes the rest, but never less than its own title line.
	const rlpTitleText = "RLP Encoded Hex String"
	minHexW := lipgloss.Width(rlpTitleText + " 📋")
	jsonW := lipgloss.Width(jsonLabel + " 📋")
	for _, line := range strings.Split(txJSON, "\n") {
		jsonW = max(jsonW, lipgloss.Width(line))
	}
	jsonW = max(1, min(jsonW, avail-minHexW))
	hexW := max(1, avail-jsonW)

	jsonCell := labelStyle.Render(jsonLabel) + " 📋\n\n" + ansi.Hardwrap(txJSON, jsonW, true)
	rlpTitle := labelStyle.Render(rlpTitleText)
	var rlpCell string
	if rlpErr != nil {
		rlpCell = rlpTitle + "\n\n" + warnStyle.Render(ansi.Hardwrap(rlpErr.Error(), hexW, true))
	} else {
		var chunks []string
		for s := rlpHex; s != ""; {
			n := min(hexW, len(s))
			chunks = append(chunks, s[:n])
			s = s[n:]
		}
		rlpCell = rlpTitle + " 📋\n\n" + strings.Join(chunks, "\n")
	}

	rendered := table.New().
		Border(lipgloss.NormalBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(styles.CBorder)).
		StyleFunc(func(row, col int) lipgloss.Style {
			return lipgloss.NewStyle().Padding(0, 1)
		}).
		Row(jsonCell, rlpCell).
		String()

	// Column bounds come from the top border's own corner/junction runes.
	m.txTableLine = -1
	top := ansi.Strip(strings.SplitN(rendered, "\n", 2)[0])
	l, mid, r := strings.Index(top, "┌"), strings.Index(top, "┬"), strings.Index(top, "┐")
	if l >= 0 && mid > l && r > mid {
		m.txTableLine = 0
		m.txTableHeight = lipgloss.Height(rendered)
		m.txTableColX = [3]int{ansi.StringWidth(top[:l]), ansi.StringWidth(top[:mid]), ansi.StringWidth(top[:r])}
	}
	return rendered
}

func (m *model) handleQRAnimTick() (tea.Model, tea.Cmd) {
	if m.activeDialog == dialogTxResult && len(m.txQRFrames) > 1 {
		m.txQRFrameIdx = (m.txQRFrameIdx + 1) % len(m.txQRFrames)
		return m, animateQRTick()
	}
	return m, nil
}

func (m *model) handleTerraClaimsCount(msg terraNullClaimsCountMsg) (tea.Model, tea.Cmd) {
	m.terraNullClaimsLoading = false
	if msg.err != nil {
		m.terraNullClaimsCount = "Error"
		m.logError(fmt.Sprintf("Terra Nullius number_of_claims failed: %s", msg.err.Error()))
	} else {
		m.terraNullClaimsCount = msg.count.String()
		m.logSuccess(fmt.Sprintf("Terra Nullius: %s total claims", msg.count.String()))
	}
	return m, nil
}

func (m *model) handleTerraClaimQuery(msg terraNullClaimQueryMsg) (tea.Model, tea.Cmd) {
	m.terraNullClaimQuerying = false
	if msg.err != nil {
		m.terraNullClaimResultErr = msg.err.Error()
		m.terraNullClaimResult = nil
		m.logError(fmt.Sprintf("Terra Nullius claims query failed: %s", msg.err.Error()))
	} else {
		m.terraNullClaimResult = msg.result
		m.terraNullClaimResultErr = ""
		m.logSuccess(fmt.Sprintf("Terra Nullius claim #%s by %s", m.terraNullClaimInput, helpers.ShortenAddr(msg.result.Claimant)))
	}
	return m, nil
}

func (m *model) handlePoolEventLine(msg poolEventLineMsg) (tea.Model, tea.Cmd) {
	if m.logBuffer != nil {
		m.logBuffer.WriteString(msg.line + "\n")
		if m.logReady {
			m.updateLogViewport()
		}
	}
	if m.poolEventMonitorActive && m.poolEventMonitor != nil {
		return m, waitForPoolEvent(m.poolEventMonitor)
	}
	return m, nil
}

// handlePoolMonitorEvent persists a monitor event off the UI thread: SQLite writes
// fsync and share the store's single connection with background queries, so a
// synchronous write here would stall input on every swap.
func (m *model) handlePoolMonitorEvent(msg poolMonitorEventMsg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	if m.poolEventMonitorActive && m.poolEventMonitor != nil {
		cmds = append(cmds, waitForPoolEventData(m.poolEventMonitor))
	}
	if m.eventStore != nil {
		cmds = append(cmds, savePoolEventCmd(m.eventStore, msg.event))
	}
	return m, tea.Batch(cmds...)
}

// handlePoolEventSaved runs after a monitor event is stored; a new pool (Initialize)
// triggers token indexing and an in-place reload of any opened pool view.
func (m *model) handlePoolEventSaved(msg poolEventSavedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.logWarn(fmt.Sprintf("[pool-monitor] db write error: %s", msg.err.Error()))
		return m, nil
	}
	ev := msg.event
	if ev.Kind != indexer.V4KindInitialize {
		return m, nil
	}
	cmds := []tea.Cmd{indexERC20TokensCmd(m.eventStore, m.rpcURL, ev.Currency0, ev.Currency1)}
	if m.poolEventMonitorActive {
		cmds = append(cmds, m.reloadPoolPager(poolViewV4Events))
	}
	if m.uniswapShowingPoolList {
		cmds = append(cmds, m.reloadPoolPager(poolViewList))
	}
	return m, tea.Batch(cmds...)
}

func (m *model) handlePoolDetails(msg poolDetailsMsg) (tea.Model, tea.Cmd) {
	shortID := shortPoolID(msg.poolID)
	if msg.err != nil {
		m.poolDetails[msg.poolID] = &poolDetailState{err: msg.err.Error()}
		m.logError(fmt.Sprintf("Pool details: failed for pool %s: %s", shortID, msg.err.Error()))
	} else {
		m.poolDetails[msg.poolID] = &poolDetailState{data: msg.data}
		m.logSuccess(fmt.Sprintf("Pool details: pool %s — tick=%d liquidity=%s", shortID, msg.data.Tick, msg.data.Liquidity))
	}
	m.refreshPoolViewports()
	return m, nil
}

func (m *model) handlePoolMonitorStopped() (tea.Model, tea.Cmd) {
	wasActive := m.poolEventMonitorActive
	m.poolEventMonitorActive = false
	m.poolEventMonitor = nil
	if wasActive {
		m.logInfo("Pool Event Monitor stopped")
	}
	return m, nil
}

func (m *model) handleV4BlockScanLine(msg v4BlockScanLineMsg) (tea.Model, tea.Cmd) {
	if m.logBuffer != nil {
		m.logBuffer.WriteString(msg.line + "\n")
		if m.logReady {
			m.updateLogViewport()
		}
	}
	if m.v4BlockScanActive && m.v4BlockScanner != nil {
		return m, waitForV4BlockScanLine(m.v4BlockScanner)
	}
	return m, nil
}

func (m *model) handleV4BlockScanDone() (tea.Model, tea.Cmd) {
	m.v4BlockScanActive = false
	m.v4BlockScanner = nil
	return m, nil
}

func (m *model) handleOscillatorBackscanLine(msg oscillatorBackscanLineMsg) (tea.Model, tea.Cmd) {
	m.logInfo(msg.line)
	if m.oscillatorBackfillActive && m.oscillatorLines != nil {
		return m, waitForOscillatorLine(m.oscillatorLines)
	}
	return m, nil
}

func (m *model) handleOscillatorBackscanDone() (tea.Model, tea.Cmd) {
	m.oscillatorBackfillActive = false
	m.oscillatorLines = nil
	m.oscillatorCancel = nil
	m.logInfo("McClellan Oscillator: backscan complete")
	return m, loadOscillatorSeriesCmd(m.eventStore, m.oscillatorBasket())
}

func (m *model) handleOscillatorSeries(msg oscillatorSeriesMsg) (tea.Model, tea.Cmd) {
	m.oscillatorDays = msg.days
	m.oscillatorSeries = msg.values
	m.oscillatorEthChange = msg.ethChange
	if msg.err != nil {
		m.oscillatorSeriesErr = msg.err.Error()
	} else {
		m.oscillatorSeriesErr = ""
	}
	return m, nil
}

func (m *model) handleIndexedEvent(msg indexedEventMsg) (tea.Model, tea.Cmd) {
	ev := msg.event
	if m.eventStore != nil {
		if err := m.eventStore.SaveEvent(ev); err != nil {
			m.logWarn(fmt.Sprintf("[indexer] db write error: %s", err.Error()))
		}
	}
	m.logInfo("[indexer] transfer detected")
	m.logIndexedEvent(ev)
	if m.txIndexerActive && m.txIndexer != nil {
		return m, waitForIndexedEvent(m.txIndexer)
	}
	return m, nil
}

func (m *model) handleIndexerStopped() (tea.Model, tea.Cmd) {
	wasActive := m.txIndexerActive
	m.txIndexerActive = false
	m.txIndexer = nil
	if wasActive {
		m.logInfo("Address indexer stopped")
	}
	return m, nil
}

func (m *model) handleV4PoolEvent(msg v4PoolEventMsg) (tea.Model, tea.Cmd) {
	ev := msg.event
	if m.eventStore != nil {
		if err := m.eventStore.SaveV4PoolEvent(ev); err != nil {
			m.logWarn(fmt.Sprintf("[v4-indexer] db write error: %s", err.Error()))
		}
	}
	m.logV4PoolEvent(ev)
	var cmds []tea.Cmd
	if m.txIndexerActive && m.txIndexer != nil {
		cmds = append(cmds, waitForV4PoolEvent(m.txIndexer))
	}
	if m.eventStore != nil && ev.Kind == indexer.V4KindInitialize {
		cmds = append(cmds, indexERC20TokensCmd(m.eventStore, m.rpcURL, ev.Currency0, ev.Currency1))
	}
	return m, tea.Batch(cmds...)
}

func (m *model) handleIndexerProgress(msg indexerProgressMsg) (tea.Model, tea.Cmd) {
	count := int64(0)
	if m.eventStore != nil {
		count, _ = m.eventStore.Count()
	}
	prefix := lipgloss.NewStyle().Foreground(styles.CAccent).Bold(true).Render("[INDEXER]")
	m.logInfo(fmt.Sprintf("%s backscan block=%d  indexed=%d records", prefix, msg.block, count))
	if m.txIndexerActive && m.txIndexer != nil {
		return m, waitForIndexerProgress(m.txIndexer)
	}
	return m, nil
}

func (m *model) handleRecentEvents(msg recentEventsMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.logWarn(fmt.Sprintf("[indexer] failed to load history: %s", msg.err.Error()))
		return m, nil
	}
	m.logInfo(fmt.Sprintf("[indexer] %d events in store — showing last %d", msg.count, len(msg.events)))
	for i := len(msg.events) - 1; i >= 0; i-- {
		m.logInfo(fmt.Sprintf("[history] event %d of %d", len(msg.events)-i, len(msg.events)))
		m.logIndexedEvent(msg.events[i])
	}
	return m, nil
}

func (m *model) handlePoolInfoResult(msg poolInfoResultMsg) (tea.Model, tea.Cmd) {
	m.poolInfoLoading = false
	m.poolInfoID = msg.poolID
	shortID := shortPoolID(msg.poolID)
	if msg.err != nil {
		m.poolInfoErr = msg.err.Error()
		m.poolInfoData = nil
		m.logError(fmt.Sprintf("Pool Info: failed for pool %s: %s", shortID, msg.err.Error()))
		return m, nil
	}
	m.poolInfoData = msg.info
	m.poolInfoErr = ""
	m.logSuccess(fmt.Sprintf("Pool Info: pool %s — sqrtPrice=%s tick=%d liquidity=%s",
		shortID, msg.info.SqrtPriceX96, msg.info.Tick, msg.info.Liquidity))
	m.poolInfoKeyLoading = true
	m.poolInfoKeyErr = ""
	m.logInfo(fmt.Sprintf("Pool Info: fetching Initialize event log for pool %s", shortID))
	return m, fetchPoolKey(m.rpcURL, msg.poolID)
}

func (m *model) handlePoolKeyResult(msg poolKeyResultMsg) (tea.Model, tea.Cmd) {
	m.poolInfoKeyLoading = false
	shortID := shortPoolID(msg.poolID)
	if msg.err != nil {
		m.poolInfoKeyErr = msg.err.Error()
		m.logWarn(fmt.Sprintf("Pool Info: Initialize event lookup failed for pool %s: %s", shortID, msg.err.Error()))
		return m, nil
	}
	if m.poolInfoData != nil {
		m.poolInfoData.Currency0 = msg.key.Currency0
		m.poolInfoData.Currency1 = msg.key.Currency1
		m.poolInfoData.Fee = msg.key.Fee
		m.poolInfoData.TickSpacing = msg.key.TickSpacing
		m.poolInfoData.Hooks = msg.key.Hooks
	}
	m.poolInfoKeyErr = ""
	m.logSuccess(fmt.Sprintf("Pool Info: pool key for %s — currency0=%s currency1=%s fee=%d",
		shortID, msg.key.Currency0, msg.key.Currency1, msg.key.Fee))
	return m, nil
}

func (m *model) handleUniswapQuote(msg uniswapQuoteMsg) (tea.Model, tea.Cmd) {
	m.uniswapEstimating = false
	if msg.err != nil {
		m.uniswapQuoteError = msg.err.Error()
		m.uniswapQuote = nil
		m.uniswapToAmount = ""
		m.uniswapFromAmount = ""
		m.uniswapPriceImpactWarn = ""
		m.uniswapHookWarn = ""
		m.logError(fmt.Sprintf("Swap quote error: %v", msg.err))
		return m, nil
	}
	m.uniswapQuoteError = ""
	m.uniswapQuote = msg.quote
	m.uniswapPriceImpactWarn = ""
	m.uniswapHookWarn = ""

	if msg.quote == nil {
		return m, nil
	}

	tokens := m.buildTokenList()
	fromToken := tokens[m.uniswapFromTokenIdx]
	toToken := tokens[m.uniswapToTokenIdx]
	isReverseQuote := m.uniswapFromAmount == "" && m.uniswapToAmount != ""

	version := "V2"
	if msg.quote.IsV3 {
		version = "V3"
	}
	if msg.quote.IsV4 {
		version = "V4"
	}

	if isReverseQuote {
		divisor := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(fromToken.Decimals)), nil))
		m.uniswapFromAmount = new(big.Float).Quo(new(big.Float).SetInt(msg.quote.AmountIn), divisor).Text('f', 6)
		m.uniswapEditingFrom = false
		m.logInfo(fmt.Sprintf("📊 %s Reverse Quote: %s → %s", version, fromToken.Symbol, toToken.Symbol))
		m.logInfo(fmt.Sprintf("  Amount In: %s %s", m.uniswapFromAmount, fromToken.Symbol))
		m.logInfo(fmt.Sprintf("  Amount Out: %s %s", m.uniswapToAmount, toToken.Symbol))
	} else {
		m.logInfo(fmt.Sprintf("📊 %s Swap Quote: %s → %s", version, fromToken.Symbol, toToken.Symbol))
		m.logInfo(fmt.Sprintf("  Amount In: %s %s", m.uniswapFromAmount, fromToken.Symbol))
		divisor := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(toToken.Decimals)), nil))
		m.uniswapToAmount = new(big.Float).Quo(new(big.Float).SetInt(msg.quote.AmountOut), divisor).Text('f', 6)
		m.logInfo(fmt.Sprintf("  Amount Out: %s %s", m.uniswapToAmount, toToken.Symbol))
	}

	m.logInfo(fmt.Sprintf("  Price Impact: %.4f%%", msg.quote.PriceImpact))
	if !msg.quote.IsV3 && msg.quote.Token0Reserve != nil && msg.quote.Token1Reserve != nil {
		divisor18 := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil))
		r0 := new(big.Float).Quo(new(big.Float).SetInt(msg.quote.Token0Reserve), divisor18)
		r1 := new(big.Float).Quo(new(big.Float).SetInt(msg.quote.Token1Reserve), divisor18)
		m.logInfo(fmt.Sprintf("  Reserves: %s / %s", r0.Text('f', 2), r1.Text('f', 2)))
	}

	if msg.quote.PriceImpact > 1.0 {
		m.uniswapPriceImpactWarn = fmt.Sprintf("⚠ High price impact: %.2f%%", msg.quote.PriceImpact)
		m.logWarn(m.uniswapPriceImpactWarn)
	} else if msg.quote.PriceImpact > 0.5 {
		m.uniswapPriceImpactWarn = fmt.Sprintf("⚠ Moderate price impact: %.2f%%", msg.quote.PriceImpact)
	}

	if msg.quote.HookAddr != (common.Address{}) {
		m.uniswapHookWarn = fmt.Sprintf("⚠ Hook-gated pool (%s) — quoting or swapping may fail if you're not allowlisted (e.g. KYC/geo restrictions)", helpers.ShortenAddr(msg.quote.HookAddr.Hex()))
		m.logWarn(m.uniswapHookWarn)
	}
	return m, nil
}

func (m *model) handleLiquidityPositions(msg liquidityPositionsMsg) (tea.Model, tea.Cmd) {
	m.liquidityLoading = false
	if msg.err != nil {
		m.liquidityErr = msg.err.Error()
		m.logError("Liquidity positions error: " + msg.err.Error())
		for _, d := range msg.diagnostics {
			m.logInfo("  " + d)
		}
	} else {
		m.liquidityPositions = msg.positions
		m.logInfo(fmt.Sprintf("V4 PositionManager: balanceOf=%d NFT(s)", msg.nftCount))
		for _, d := range msg.diagnostics {
			m.logInfo("  " + d)
		}
		if len(msg.positions) == 0 {
			m.logInfo("No active V4 liquidity positions found")
		} else {
			m.logInfo(fmt.Sprintf("%d position(s) with active liquidity", len(msg.positions)))
		}
	}
	return m, nil
}

func (m *model) handleENSLookupResult(msg ensLookupResultMsg) (tea.Model, tea.Cmd) {
	m.ensLookupActive = false
	if msg.debugInfo != "" {
		m.logInfo(fmt.Sprintf("ENS debug: %s", msg.debugInfo))
	}
	if msg.err == nil && msg.ensName != "" && msg.address == m.ensLookupAddr {
		if strings.TrimSpace(m.nicknameInput.Value()) == "" {
			m.nicknameInput.SetValue(msg.ensName)
		}
		m.logSuccess(fmt.Sprintf("Found ENS name: %s", msg.ensName))
	} else if msg.err != nil && msg.address == m.ensLookupAddr {
		m.logError(fmt.Sprintf("ENS lookup error: %v", msg.err))
	} else if msg.address == m.ensLookupAddr {
		m.logInfo("No ENS name found for address: " + helpers.FadeString(helpers.ShortenAddr(msg.address), "#F25D94", "#EDFF82"))
	}
	return m, nil
}

func (m *model) handleENSForwardResolve(msg ensForwardResolveMsg) (tea.Model, tea.Cmd) {
	m.ensLookupActive = false
	if msg.debugInfo != "" {
		m.logInfo(fmt.Sprintf("ENS resolve debug: %s", msg.debugInfo))
	}
	if msg.err == nil && msg.address != "" {
		m.input.SetValue(msg.address)
		if strings.TrimSpace(m.nicknameInput.Value()) == "" {
			m.nicknameInput.SetValue(msg.ensName)
		}
		m.focusedInput = 1
		m.input.Blur()
		m.nicknameInput.Focus()
		m.logSuccess(fmt.Sprintf("Resolved %s to %s", msg.ensName, helpers.ShortenAddr(msg.address)))
	} else if msg.err != nil {
		m.logError(fmt.Sprintf("ENS resolution error: %v", msg.err))
		m.addError = fmt.Sprintf("Failed to resolve %s", msg.ensName)
		m.addErrTime = time.Now()
	}
	return m, nil
}

// shortPoolID shortens a pool ID hex string for display in log messages.
func shortPoolID(id string) string {
	if len(id) > 16 {
		return id[:10] + "…" + id[len(id)-6:]
	}
	return id
}
