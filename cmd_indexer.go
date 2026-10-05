package main

import (
	"context"
	"time"

	"charm-wallet-tui/helpers"
	"charm-wallet-tui/indexer"
	"charm-wallet-tui/store"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ethereum/go-ethereum/common"
)

// indexERC20TokensCmd looks up name/symbol/decimals for each address and stores results.
// Addresses already in the table are skipped. Errors are silently swallowed.
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

func waitForIndexedEvent(idx *indexer.Indexer) tea.Cmd {
	return func() tea.Msg {
		event, ok := <-idx.Events()
		if !ok {
			return indexerStoppedMsg{}
		}
		return indexedEventMsg{event: event}
	}
}

func waitForV4PoolEvent(idx *indexer.Indexer) tea.Cmd {
	return func() tea.Msg {
		event, ok := <-idx.PoolEvents()
		if !ok {
			return v4PoolIndexerStoppedMsg{}
		}
		return v4PoolEventMsg{event: event}
	}
}

func waitForIndexerProgress(idx *indexer.Indexer) tea.Cmd {
	return func() tea.Msg {
		block, ok := <-idx.Progress()
		if !ok {
			return nil
		}
		return indexerProgressMsg{block: block}
	}
}

func waitForV4BlockScanLine(scanner *helpers.V4BlockScanner) tea.Cmd {
	return func() tea.Msg {
		line, ok := <-scanner.Lines()
		if !ok {
			return v4BlockScanDoneMsg{}
		}
		return v4BlockScanLineMsg{line: line}
	}
}

func loadPoolPageCmd(s *store.Store, view poolViewKind, search string, offset, limit, seq int) tea.Cmd {
	return func() tea.Msg {
		rows, total, err := s.V4PoolStatsPage(search, limit, offset)
		return poolPageMsg{view: view, seq: seq, offset: offset, rows: rows, total: total, err: err}
	}
}

func savePoolEventCmd(s *store.Store, ev indexer.V4PoolEvent) tea.Cmd {
	return func() tea.Msg {
		return poolEventSavedMsg{event: ev, err: s.SaveV4PoolEvent(ev)}
	}
}

func waitForPoolEventData(monitor *helpers.PoolEventMonitor) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-monitor.Events()
		if !ok {
			return poolEventMonitorStoppedMsg{}
		}
		return poolMonitorEventMsg{event: ev}
	}
}

func waitForPoolEvent(monitor *helpers.PoolEventMonitor) tea.Cmd {
	return func() tea.Msg {
		line, ok := <-monitor.Lines()
		if !ok {
			return poolEventMonitorStoppedMsg{}
		}
		return poolEventLineMsg{line: line}
	}
}
