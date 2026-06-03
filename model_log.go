package main

import (
	"fmt"
	"math/big"
	"strings"

	"charm-wallet-tui/helpers"
	"charm-wallet-tui/indexer"
	"charm-wallet-tui/styles"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/log"
	"github.com/ethereum/go-ethereum/common"
)

// Pre-computed log colorize styles — allocated once at startup.
var (
	logColorError = lipgloss.NewStyle().Foreground(styles.CError).Bold(true)
	logColorInfo  = lipgloss.NewStyle().Foreground(styles.CAccent).Bold(true)
	logColorDebug = lipgloss.NewStyle().Foreground(styles.CAccent2).Bold(true)
	logColorCheck = lipgloss.NewStyle().Foreground(styles.CAccent)
)

func (m *model) logInfo(msg string)    { m.logAt(func(s string) { m.logger.Info(s) }, msg) }
func (m *model) logSuccess(msg string) { m.logAt(func(s string) { m.logger.Info("✓", "msg", s) }, msg) }
func (m *model) logError(msg string)   { m.logAt(func(s string) { m.logger.Error(s) }, msg) }
func (m *model) logWarn(msg string)    { m.logAt(func(s string) { m.logger.Warn(s) }, msg) }
func (m *model) logDebug(msg string)   { m.logAt(func(s string) { m.logger.Debug(s) }, msg) }

func (m *model) logAt(fn func(string), msg string) {
	if !m.logEnabled || !m.logReady || m.logger == nil {
		return
	}
	fn(msg)
	m.updateLogViewport()
}

// logIndexedEvent logs a single IndexedEvent in a detailed, structured format.
func (m *model) logIndexedEvent(ev indexer.IndexedEvent) {
	divisor := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(ev.Decimals)), nil))
	humanAmt := new(big.Float).Quo(new(big.Float).SetInt(ev.Value), divisor)
	m.logInfo(fmt.Sprintf("  Token   : %s (%s)", ev.Symbol, helpers.HyperAddr(ev.Token)))
	m.logInfo(fmt.Sprintf("  Block   : %d", ev.Block))
	m.logInfo(fmt.Sprintf("  TxHash  : %s", helpers.HyperTxHash(ev.TxHash)))
	m.logInfo(fmt.Sprintf("  LogIndex: %d", ev.LogIndex))
	m.logInfo(fmt.Sprintf("  From    : %s", helpers.HyperAddr(ev.From)))
	m.logInfo(fmt.Sprintf("  To      : %s", helpers.HyperAddr(ev.To)))
	m.logInfo(fmt.Sprintf("  Value   : %s raw  (%s %s)", ev.Value.String(), fmt.Sprintf("%.6f", humanAmt), ev.Symbol))
	m.logInfo(fmt.Sprintf("  Decimals: %d", ev.Decimals))
	m.logInfo("  ─────────────────────────────────────────────────────────")
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
		m.logInfo(fmt.Sprintf("[V4-SWAP] %s  Pool: %s", dir, shortPool(ev.PoolID)))
		m.logInfo(fmt.Sprintf("  Sender    : %s", helpers.HyperAddr(ev.Sender)))
		m.logInfo(fmt.Sprintf("  Amount0   : %s", bigStr(ev.Amount0)))
		m.logInfo(fmt.Sprintf("  Amount1   : %s", bigStr(ev.Amount1)))
		m.logInfo(fmt.Sprintf("  Tick      : %s", bigStr(ev.Tick)))
		m.logInfo(fmt.Sprintf("  Block     : %d", ev.Block))
		m.logInfo(fmt.Sprintf("  TxHash    : %s", helpers.HyperTxHash(ev.TxHash)))
		m.logInfo(sep)

	case indexer.V4KindModifyLiquidity:
		sign := "+"
		if ev.LiquidityDelta != nil && ev.LiquidityDelta.Sign() < 0 {
			sign = "-"
		}
		m.logInfo(fmt.Sprintf("[V4-LIQ] %sΔ  Pool: %s", sign, shortPool(ev.PoolID)))
		m.logInfo(fmt.Sprintf("  Sender    : %s", helpers.HyperAddr(ev.Sender)))
		m.logInfo(fmt.Sprintf("  ΔLiquidity: %s", bigStr(ev.LiquidityDelta)))
		m.logInfo(fmt.Sprintf("  Ticks     : [%s, %s]", bigStr(ev.TickLower), bigStr(ev.TickUpper)))
		m.logInfo(fmt.Sprintf("  Block     : %d", ev.Block))
		m.logInfo(fmt.Sprintf("  TxHash    : %s", helpers.HyperTxHash(ev.TxHash)))
		m.logInfo(sep)

	case indexer.V4KindDonate:
		m.logInfo(fmt.Sprintf("[V4-DONATE]  Pool: %s", shortPool(ev.PoolID)))
		m.logInfo(fmt.Sprintf("  Sender  : %s", helpers.HyperAddr(ev.Sender)))
		m.logInfo(fmt.Sprintf("  Amount0 : %s", bigStr(ev.Amount0)))
		m.logInfo(fmt.Sprintf("  Amount1 : %s", bigStr(ev.Amount1)))
		m.logInfo(fmt.Sprintf("  Block   : %d", ev.Block))
		m.logInfo(fmt.Sprintf("  TxHash  : %s", helpers.HyperTxHash(ev.TxHash)))
		m.logInfo(sep)

	case indexer.V4KindTransfer:
		m.logInfo(fmt.Sprintf("[V4-TRANSFER]  TokenID: %s", bigStr(ev.TokenID)))
		m.logInfo(fmt.Sprintf("  From    : %s", helpers.HyperAddr(ev.From)))
		m.logInfo(fmt.Sprintf("  To      : %s", helpers.HyperAddr(ev.To)))
		m.logInfo(fmt.Sprintf("  Amount  : %s", bigStr(ev.Amount0)))
		m.logInfo(fmt.Sprintf("  Block   : %d", ev.Block))
		m.logInfo(fmt.Sprintf("  TxHash  : %s", helpers.HyperTxHash(ev.TxHash)))
		m.logInfo(sep)
	}
}

// colorizeLogContent applies color coding to log level keywords and check marks.
func colorizeLogContent(content string) string {
	if content == "" {
		return content
	}
	var result strings.Builder
	for i, line := range strings.Split(content, "\n") {
		if i > 0 {
			result.WriteString("\n")
		}
		line = strings.ReplaceAll(line, "✓", logColorCheck.Render("✓"))
		switch {
		case strings.Contains(line, " ERROR"):
			line = strings.Replace(line, " ERROR", " "+logColorError.Render("ERROR"), 1)
		case strings.Contains(line, " INFO"):
			line = strings.Replace(line, " INFO", " "+logColorInfo.Render("INFO"), 1)
		case strings.Contains(line, " DEBUG"):
			line = strings.Replace(line, " DEBUG", " "+logColorDebug.Render("DEBUG"), 1)
		}
		result.WriteString(line)
	}
	return result.String()
}

// maxLogBytes is the maximum size of the in-memory log buffer (~2 MB).
const maxLogBytes = 2 * 1024 * 1024

// updateLogViewport refreshes log viewport content, preserving the scroll position
// unless the viewport was already at the bottom.
func (m *model) updateLogViewport() {
	if !m.logReady || m.logBuffer == nil {
		return
	}
	atBottom := m.logViewport.AtBottom()
	content := m.logBuffer.String()
	if len(content) > maxLogBytes {
		trimmed := content[len(content)-maxLogBytes:]
		if idx := strings.Index(trimmed, "\n"); idx >= 0 {
			trimmed = trimmed[idx+1:]
		}
		m.logBuffer.Reset()
		m.logBuffer.WriteString(trimmed)
		content = trimmed
	}
	m.logViewport.SetContent(colorizeLogContent(content))
	if atBottom {
		m.logViewport.GotoBottom()
	}
}

// initLogger initialises the charmbracelet logger that writes into the log buffer.
func (m *model) initLogger() {
	m.logger = log.NewWithOptions(m.logBuffer, log.Options{
		ReportTimestamp: true,
		TimeFormat:      "15:04:05",
	})
	m.logger.SetLevel(log.DebugLevel)
	m.logger.SetStyles(&log.Styles{
		Timestamp: lipgloss.NewStyle().Foreground(styles.CMuted),
		Caller:    lipgloss.NewStyle().Faint(true),
		Prefix:    lipgloss.NewStyle().Bold(true).Foreground(styles.CAccent2),
		Message:   lipgloss.NewStyle().Foreground(styles.CText),
		Key:       lipgloss.NewStyle().Foreground(styles.CAccent),
		Value:     lipgloss.NewStyle().Foreground(styles.CText),
		Separator: lipgloss.NewStyle().Faint(true),
		Levels: map[log.Level]lipgloss.Style{
			log.DebugLevel: lipgloss.NewStyle().Foreground(styles.CBorder).Bold(true).SetString("DEBUG"),
			log.InfoLevel:  lipgloss.NewStyle().Foreground(styles.CAccent).Bold(true).SetString("INFO"),
			log.WarnLevel:  lipgloss.NewStyle().Foreground(styles.CWarn).Bold(true).SetString("WARN"),
			log.ErrorLevel: lipgloss.NewStyle().Foreground(styles.CError).Bold(true).SetString("ERROR"),
		},
	})
	m.logReady = true
	m.logInfo("Logger enabled")
}
