package store

import (
	"fmt"
	"path/filepath"
	"testing"
)

func TestV4PoolStatsPage(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := s.db.Exec(q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	exec(`INSERT INTO erc20_tokens(address, symbol, name, decimals) VALUES
		('0xa', 'WETH', 'Wrapped Ether', 18), ('0xb', 'USDC', 'USD Coin', 6), ('0xc', 'DAI', 'Dai', 18), ('0xd', 'my_tok', 'Underscore', 18)`)
	// 25 pools: pool i is newer than pool i-1. Pools 0–4 are WETH/USDC, 5 is DAI/WETH, 6 is my_tok/DAI, rest DAI/DAI.
	for i := 0; i < 25; i++ {
		c0, c1 := "0xc", "0xc"
		switch {
		case i < 5:
			c0, c1 = "0xa", "0xb"
		case i == 5:
			c0, c1 = "0xc", "0xa"
		case i == 6:
			c0, c1 = "0xd", "0xc"
		}
		exec(`INSERT INTO v4_pools(pool_id, block, tx_hash, log_index, currency0, currency1, fee, tick_spacing, hooks, sqrt_price, init_tick, seen_at)
			VALUES (?, ?, '0x', 0, ?, ?, 500, 10, '0x0', '0', 0, ?)`,
			fmt.Sprintf("p%02d", i), i, c0, c1, fmt.Sprintf("2026-01-01 00:00:%02d", i))
	}
	// Pool p00: 3 swaps (|amount0| sum = 6) and 2 liquidity events (|delta| sum = 30).
	for i, a := range []string{"1", "-2", "3"} {
		exec(`INSERT INTO v4_swaps(block, tx_hash, log_index, pool_id, sender, amount0, amount1, sqrt_price, liquidity, tick, fee)
			VALUES (0, 'tx', ?, 'p00', '0x', ?, '0', '0', '0', 0, 0)`, i, a)
	}
	for i, d := range []string{"10", "-20"} {
		exec(`INSERT INTO v4_modify_liquidity(block, tx_hash, log_index, pool_id, sender, tick_lower, tick_upper, liq_delta, salt)
			VALUES (0, 'ml', ?, 'p00', '0x', 0, 0, ?, '0x')`, i, d)
	}

	page, total, err := s.V4PoolStatsPage("", 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 25 || len(page) != 20 || page[0].PoolID != "p24" || page[19].PoolID != "p05" {
		t.Fatalf("page 1: total=%d len=%d first=%s last=%s", total, len(page), page[0].PoolID, page[len(page)-1].PoolID)
	}
	page, _, _ = s.V4PoolStatsPage("", 20, 20)
	if len(page) != 5 || page[0].PoolID != "p04" || page[4].PoolID != "p00" {
		t.Fatalf("page 2: len=%d", len(page))
	}
	p0 := page[4]
	if p0.Swaps != 3 || p0.LiqEvents != 2 || p0.SwapVolume0 != 6 || p0.LiqVolume != 30 || p0.TickSpacing != 10 {
		t.Errorf("p00 aggregates: swaps=%d liq=%d vol0=%v liqVol=%v ts=%d (volumes must not be multiplied by the joined row count)",
			p0.Swaps, p0.LiqEvents, p0.SwapVolume0, p0.LiqVolume, p0.TickSpacing)
	}

	for _, c := range []struct {
		search string
		want   int
	}{
		{"usdc", 5},
		{"weth", 6},
		{"WETH/USDC", 5},
		{"usdc/weth", 5},
		{"weth/dai", 1},
		{"dai/", 20},
		{"_", 1},
		{"%", 0},
		{"nope", 0},
	} {
		page, total, err := s.V4PoolStatsPage(c.search, 20, 0)
		if err != nil {
			t.Fatalf("%q: %v", c.search, err)
		}
		if total != c.want || len(page) != min(c.want, 20) {
			t.Errorf("search %q: total=%d len=%d, want %d", c.search, total, len(page), c.want)
		}
	}
}
