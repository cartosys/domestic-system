package helpers

import (
	"context"
	"flag"
	"fmt"
	"testing"
	"time"
)

var (
	backfillFrom = flag.Uint64("backfill.from", 24686487, "start block for TestBackfillPoolEvents_LocalNode")
	backfillTo   = flag.Uint64("backfill.to", 24686489, "end block for TestBackfillPoolEvents_LocalNode")
	backfillNode = flag.String("backfill.node", "http://localhost:8545", "RPC node URL for TestBackfillPoolEvents_LocalNode")
)

// TestBackfillPoolEvents_LocalNode scans a block range for Uniswap V4
// PoolManager events against a configurable RPC node.
//
// Default range is 24686487–24686489 against http://localhost:8545.
// Override with flags:
//
//	go test ./helpers -v -run TestBackfillPoolEvents_LocalNode -args \
//	  -backfill.node=https://eth.llamarpc.com \
//	  -backfill.from=21688000 -backfill.to=21690000
func TestBackfillPoolEvents_LocalNode(t *testing.T) {
	rpcURL := *backfillNode
	from := *backfillFrom
	to := *backfillTo

	if from > to {
		t.Fatalf("backfill.from (%d) > backfill.to (%d)", from, to)
	}

	blockCount := to - from + 1
	timeout := 30*time.Second + time.Duration(blockCount/1000)*time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	t.Logf("node: %s", rpcURL)
	t.Logf("scanning blocks %d–%d (%d blocks)", from, to, blockCount)

	ch := BackfillPoolEvents(ctx, rpcURL, from, to)

	var lines []string
	for line := range ch {
		t.Log(line)
		lines = append(lines, line)
	}

	if len(lines) == 0 {
		t.Fatal("no lines received — channel closed immediately, possible dial or RPC error")
	}

	if lines[0] == "" {
		t.Error("expected non-empty first line (scan header)")
	}

	last := lines[len(lines)-1]
	want := fmt.Sprintf("[Backfill] done — scanned %d blocks", blockCount)
	if last != want {
		t.Errorf("unexpected final line:\n  got:  %q\n  want: %q", last, want)
	}

	t.Logf("total lines: %d", len(lines))
}
