# Uniswap Integration

## Overview
Successfully integrated Uniswap V2 swap price quotes into the TUI wallet's Swap view.

## Implementation

### New Files
- **helpers/uniswap.go** - Uniswap helper functions for fetching swap quotes via RPC
- **helpers/uniswap_test.go** - Comprehensive tests for Uniswap functionality
- **helpers/UNISWAP.md** - Documentation for the Uniswap helper functions

### Modified Files
- **main.go** - Added quote fetching logic and UI integration
- **views/uniswap/uniswap.go** - Updated view to display price impact warnings

## Features

### Automatic Quote Fetching
When the user:
1. Enters a "From" amount > 0
2. Selects a "To" currency

The system automatically:
- Fetches real-time swap quotes from Uniswap V2 pairs via RPC
- Calculates the expected "To" amount
- Displays price impact warnings for high slippage

### Price Impact Warnings
- **Moderate** (0.5-1.0%): Orange warning displayed below "To" field
- **High** (>1.0%): Orange warning with explicit alert

### Logging
All Uniswap data is logged to the logger view with:
- 📊 Swap quote header showing token pair
- Amount in (user's input)
- Amount out (calculated output)
- Price impact percentage
- Reserve amounts for both tokens

### Supported Pairs
Currently supports:
- USDC ↔ ETH (WETH)

## User Experience

### Visual Feedback
1. User types amount in "From" field
2. System shows "Estimating..." in "To" field
3. Once quote is fetched:
   - "To" amount is calculated and displayed
   - Price impact warning appears below in orange if significant
   - All details are logged to the log panel

### Example Flow
```
From: ETH
Balance: 1.234567
[1.0]

⬇

To: USDC  
Balance: 0.000000
[2328.065800]

⚠ Moderate price impact: 0.32%

[Swap Button]
```

## Technical Details

### Quote Calculation
Uses Uniswap V2 constant product formula:
```
amountOut = (amountIn × 997 × reserveOut) / (reserveIn × 1000 + amountIn × 997)
```

The 997/1000 factor accounts for the 0.3% trading fee.

### RPC Calls
Makes direct contract calls to:
- `token0()` - Get first token in pair
- `token1()` - Get second token in pair
- `getReserves()` - Get current liquidity reserves

### Price Impact Formula
```
priceImpact = (spotPrice - effectivePrice) / spotPrice × 100
```

Where:
- spotPrice = reserveOut / reserveIn (before swap)
- effectivePrice = amountOut / amountIn (actual execution price)

## Testing

Run tests with:
```bash
ETH_RPC_URL="https://eth.drpc.org" go test ./helpers -v -run "TestGetSwapQuote|TestGetUniswapV2Pair"
```

All tests pass ✅

## Future Enhancements

Potential improvements:
- Support for more token pairs (DAI, USDT, etc.)
- Uniswap V3 integration
- Slippage tolerance settings
- Multi-hop routing for better prices
- Real-time quote updates
- Gas estimation



                                                                                                                                                                             Sqlite indexer tables:                                    
  ┌─────────────────────┬──────────────────┬────────────────────┬────────────────────────────────────────────────────────┐                                          
  │        Table        │        PK        │        FKs         │                        Purpose                         │                                                                                         
  ├─────────────────────┼──────────────────┼────────────────────┼────────────────────────────────────────────────────────┤                                         
  │ v4_pools            │ pool_id (TEXT)   │ —                  │ One row per Initialize event                           │                                                                                         
  ├─────────────────────┼──────────────────┼────────────────────┼────────────────────────────────────────────────────────┤
  │ v4_swaps            │ id AUTOINCREMENT │ pool_id → v4_pools │ Swap events                                            │                                                                                         
  ├─────────────────────┼──────────────────┼────────────────────┼────────────────────────────────────────────────────────┤                                                 
  │ v4_modify_liquidity │ id AUTOINCREMENT │ pool_id → v4_pools │ ModifyLiquidity events                                 │                                                                                         
  ├─────────────────────┼──────────────────┼────────────────────┼────────────────────────────────────────────────────────┤                                              
  │ v4_donates          │ id AUTOINCREMENT │ pool_id → v4_pools │ Donate events                                          │                                                                                       
  ├─────────────────────┼──────────────────┼────────────────────┼────────────────────────────────────────────────────────┤                                            
  │ v4_transfers        │ id AUTOINCREMENT │ —                  │ ERC-6909 transfers, indexed by caller/from/to/token_id │                                                                                       
  └─────────────────────┴──────────────────┴────────────────────┴────────────────────────────────────────────────────────┘           

  sqlite3 ~/.charm-wallet-events.db "
  SELECT
    p.pool_id,
    t0.symbol AS token0, t0.name AS name0,
    p.currency0 as name0_address,
    t1.symbol AS token1, t1.name AS name1,
    p.currency1 as name1_address,
    p.fee,
    COUNT(DISTINCT s.id)  AS swaps,
    COUNT(DISTINCT ml.id) AS liq_events,
    p.seen_at
  FROM v4_pools p
  LEFT JOIN erc20_tokens        t0 ON t0.address = p.currency0
  LEFT JOIN erc20_tokens        t1 ON t1.address = p.currency1
  LEFT JOIN v4_swaps            s  ON s.pool_id  = p.pool_id
  LEFT JOIN v4_modify_liquidity ml ON ml.pool_id = p.pool_id
  GROUP BY p.pool_id
  ORDER BY swaps DESC;
  "

or use rindexer ;) https://rindexer.xyz/