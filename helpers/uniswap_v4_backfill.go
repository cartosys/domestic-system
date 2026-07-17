package helpers

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"sync"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

// backfillChunkSize is the maximum block range per eth_getLogs request.
// Reth's default cap is 20 000 blocks; staying well under avoids rejections.
const backfillChunkSize = uint64(10_000)

// BackfillPoolEvents queries the Uniswap V4 PoolManager for all events in the
// inclusive block range [fromBlock, toBlock] using eth_getLogs over HTTP. Each
// formatted event line is sent on the returned channel; the channel is closed
// when all chunks have been processed or a fatal error occurs.
//
// The httpURL must use the http:// or https:// scheme (not WebSocket).
// Large ranges are split into backfillChunkSize-block chunks automatically.
func BackfillPoolEvents(ctx context.Context, httpURL string, fromBlock, toBlock uint64) <-chan string {
	out := make(chan string, 512)
	go func() {
		defer close(out)

		emit := func(s string) {
			select {
			case out <- s:
			case <-ctx.Done():
			}
		}

		if httpURL == "" {
			emit("[Backfill] ERROR: no RPC URL provided")
			return
		}
		// Reject WebSocket URLs — eth_getLogs requires HTTP.
		if strings.HasPrefix(httpURL, "ws://") || strings.HasPrefix(httpURL, "wss://") {
			emit("[Backfill] ERROR: HTTP URL required for backfill, got WebSocket URL")
			return
		}
		if fromBlock > toBlock {
			emit(fmt.Sprintf("[Backfill] ERROR: fromBlock (%d) > toBlock (%d)", fromBlock, toBlock))
			return
		}

		client, err := ethclient.DialContext(ctx, httpURL)
		if err != nil {
			emit(fmt.Sprintf("[Backfill] ERROR: dial %s: %v", httpURL, err))
			return
		}
		defer client.Close()

		parsedABI, err := abi.JSON(strings.NewReader(poolManagerEventsABI))
		if err != nil {
			emit(fmt.Sprintf("[Backfill] ERROR: parse ABI: %v", err))
			return
		}

		eventNames := make(map[common.Hash]string, len(parsedABI.Events))
		allSigs := make([]common.Hash, 0, len(parsedABI.Events))
		for name, ev := range parsedABI.Events {
			eventNames[ev.ID] = name
			allSigs = append(allSigs, ev.ID)
		}

		poolManager := addressesForClient(ctx, client).V4PoolManager
		var (
			mu       sync.RWMutex
			poolKeys = make(map[common.Hash]v4PoolKey)
			syms     = newV4SymbolCache()
		)

		totalBlocks := toBlock - fromBlock + 1
		emit(fmt.Sprintf("[Backfill] scanning blocks %d–%d (%d blocks) against PoolManager %s",
			fromBlock, toBlock, totalBlocks, poolManager.Hex()))

		chunkStart := fromBlock
		for chunkStart <= toBlock {
			if ctx.Err() != nil {
				return
			}

			chunkEnd := chunkStart + backfillChunkSize - 1
			if chunkEnd > toBlock {
				chunkEnd = toBlock
			}

			query := ethereum.FilterQuery{
				FromBlock: new(big.Int).SetUint64(chunkStart),
				ToBlock:   new(big.Int).SetUint64(chunkEnd),
				Addresses: []common.Address{poolManager},
				Topics:    [][]common.Hash{allSigs},
			}

			logs, err := client.FilterLogs(ctx, query)
			if err != nil {
				emit(fmt.Sprintf("[Backfill] ERROR: eth_getLogs blocks %d–%d: %v", chunkStart, chunkEnd, err))
				return
			}

			emit(fmt.Sprintf("[Backfill] blocks %d–%d → %d events", chunkStart, chunkEnd, len(logs)))

			for _, lg := range logs {
				line, fmtErr := v4FormatLog(&parsedABI, lg, eventNames, &mu, poolKeys, ctx, client, syms)
				if fmtErr != nil {
					emit(fmt.Sprintf("[Backfill] decode error: %v", fmtErr))
				} else if line != "" {
					emit(line)
				}
			}

			chunkStart = chunkEnd + 1
		}

		emit(fmt.Sprintf("[Backfill] done — scanned %d blocks", totalBlocks))
	}()
	return out
}
