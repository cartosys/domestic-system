package store

import (
	"context"
	"fmt"
	"strings"

	"charm-wallet-tui/indexer"

	"github.com/ethereum/go-ethereum/ethclient"
)

const backfillChunk = uint64(10_000)

// IndexV4Backfill scans the Uniswap V4 PoolManager for all events in
// [fromBlock, toBlock] and saves each one to the appropriate typed table.
// Progress and error messages are streamed on the returned channel, which
// is closed when the scan completes or a fatal error occurs.
//
// The httpURL must use the http:// or https:// scheme (not WebSocket).
// Large ranges are split into backfillChunk-block chunks automatically.
//
// Initialize events are saved first within each chunk so that pool rows exist
// before their dependent Swap / ModifyLiquidity / Donate rows are inserted.
func (s *Store) IndexV4Backfill(ctx context.Context, httpURL string, fromBlock, toBlock uint64) <-chan string {
	out := make(chan string, 512)
	go func() {
		defer close(out)

		emit := func(msg string) {
			select {
			case out <- msg:
			case <-ctx.Done():
			}
		}

		if strings.HasPrefix(httpURL, "ws://") || strings.HasPrefix(httpURL, "wss://") {
			emit("[IndexBackfill] ERROR: HTTP URL required, got WebSocket URL")
			return
		}
		if fromBlock > toBlock {
			emit(fmt.Sprintf("[IndexBackfill] ERROR: fromBlock (%d) > toBlock (%d)", fromBlock, toBlock))
			return
		}

		client, err := ethclient.DialContext(ctx, httpURL)
		if err != nil {
			emit(fmt.Sprintf("[IndexBackfill] ERROR: dial %s: %v", httpURL, err))
			return
		}
		defer client.Close()

		totalBlocks := toBlock - fromBlock + 1
		emit(fmt.Sprintf("[IndexBackfill] scanning blocks %d–%d (%d blocks)", fromBlock, toBlock, totalBlocks))

		var totalSaved int
		for chunkStart := fromBlock; chunkStart <= toBlock; {
			if ctx.Err() != nil {
				return
			}
			chunkEnd := chunkStart + backfillChunk - 1
			if chunkEnd > toBlock {
				chunkEnd = toBlock
			}

			events, err := indexer.FetchAllV4PoolEvents(ctx, client, chunkStart, chunkEnd)
			if err != nil {
				emit(fmt.Sprintf("[IndexBackfill] ERROR: fetch blocks %d–%d: %v", chunkStart, chunkEnd, err))
				return
			}

			// Save Initialize events first so pool rows exist before FK-referencing rows.
			saved := 0
			for _, ev := range events {
				if ev.Kind == indexer.V4KindInitialize {
					if err := s.SaveV4PoolEvent(ev); err != nil {
						emit(fmt.Sprintf("[IndexBackfill] WARN: save initialize: %v", err))
					} else {
						saved++
					}
				}
			}
			for _, ev := range events {
				if ev.Kind != indexer.V4KindInitialize {
					if err := s.SaveV4PoolEvent(ev); err != nil {
						emit(fmt.Sprintf("[IndexBackfill] WARN: save %s: %v", ev.Kind, err))
					} else {
						saved++
					}
				}
			}

			totalSaved += saved
			emit(fmt.Sprintf("[IndexBackfill] blocks %d–%d → %d events (%d fetched)",
				chunkStart, chunkEnd, saved, len(events)))

			chunkStart = chunkEnd + 1
		}

		emit(fmt.Sprintf("[IndexBackfill] done — %d blocks, %d events indexed", totalBlocks, totalSaved))
	}()
	return out
}
