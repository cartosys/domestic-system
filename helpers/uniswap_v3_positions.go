package helpers

import (
	"context"
	"fmt"
	"math"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

// v3NftPositionManager is the Uniswap V3 NonfungiblePositionManager on mainnet.
var v3NftPositionManager = common.HexToAddress("0xC36442b4a4522E871399CD717aBDD847Ab11FE88")

// LiquidityPosition represents a Uniswap V3 NFT liquidity position.
type LiquidityPosition struct {
	TokenID        *big.Int
	Token0         common.Address
	Token1         common.Address
	Token0Symbol   string
	Token1Symbol   string
	Token0Decimals uint8
	Token1Decimals uint8
	Fee            uint32   // fee tier in hundredths of a bip (3000 = 0.3%)
	TickLower      int32
	TickUpper      int32
	Liquidity      *big.Int
	TokensOwed0    *big.Int // uncollected fees in token0 base units
	TokensOwed1    *big.Int // uncollected fees in token1 base units
	MinPrice       float64  // human-readable price of token0 in token1 at tickLower
	MaxPrice       float64  // human-readable price of token0 in token1 at tickUpper
}

// GetLiquidityPositions fetches all Uniswap V3 NFT positions held by ownerAddr.
// Caps at 50 to avoid excessive RPC calls.
func GetLiquidityPositions(rpcURL string, ownerAddr common.Address) ([]LiquidityPosition, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := ethclient.DialContext(ctx, rpcURL)
	if err != nil {
		return nil, fmt.Errorf("dial: %w", err)
	}
	defer client.Close()

	balance, err := v3BalanceOf(ctx, client, ownerAddr)
	if err != nil {
		return nil, fmt.Errorf("balanceOf: %w", err)
	}
	if balance == 0 {
		return nil, nil
	}

	count := min(balance, 50)

	syms := newV4SymbolCache()
	positions := make([]LiquidityPosition, 0, count)

	for i := uint64(0); i < count; i++ {
		tokenId, err := v3TokenOfOwnerByIndex(ctx, client, ownerAddr, i)
		if err != nil {
			continue
		}
		pos, err := v3FetchPosition(ctx, client, tokenId)
		if err != nil {
			continue
		}

		// Skip positions with no liquidity and no fees
		if pos.Liquidity.Sign() == 0 &&
			pos.TokensOwed0.Sign() == 0 &&
			pos.TokensOwed1.Sign() == 0 {
			continue
		}

		pos.Token0Symbol = syms.getOrFetch(ctx, client, pos.Token0)
		if pos.Token0Symbol == "" {
			pos.Token0Symbol = pos.Token0.Hex()[:8]
		}
		pos.Token1Symbol = syms.getOrFetch(ctx, client, pos.Token1)
		if pos.Token1Symbol == "" {
			pos.Token1Symbol = pos.Token1.Hex()[:8]
		}

		pos.Token0Decimals = v3ERC20Decimals(ctx, client, pos.Token0)
		pos.Token1Decimals = v3ERC20Decimals(ctx, client, pos.Token1)

		pos.MinPrice = v3TickToPrice(pos.TickLower, pos.Token0Decimals, pos.Token1Decimals)
		pos.MaxPrice = v3TickToPrice(pos.TickUpper, pos.Token0Decimals, pos.Token1Decimals)

		positions = append(positions, pos)
	}

	return positions, nil
}

// v3TickToPrice converts a Uniswap V3 tick to a human-readable price (token1 per token0).
func v3TickToPrice(tick int32, decimals0, decimals1 uint8) float64 {
	rawPrice := math.Pow(1.0001, float64(tick))
	decimalAdjust := math.Pow(10, float64(int(decimals0)-int(decimals1)))
	return rawPrice * decimalAdjust
}

// v3BalanceOf calls balanceOf(address) on the NFT Position Manager.
func v3BalanceOf(ctx context.Context, client *ethclient.Client, owner common.Address) (uint64, error) {
	// selector: balanceOf(address) = 0x70a08231
	data := make([]byte, 36)
	copy(data[:4], []byte{0x70, 0xa0, 0x82, 0x31})
	// address is right-aligned within 32-byte slot starting at data[4]
	copy(data[16:36], owner.Bytes())

	result, err := client.CallContract(ctx, ethereum.CallMsg{To: &v3NftPositionManager, Data: data}, nil)
	if err != nil {
		return 0, err
	}
	if len(result) < 32 {
		return 0, fmt.Errorf("short response: %d bytes", len(result))
	}
	return new(big.Int).SetBytes(result[:32]).Uint64(), nil
}

// v3TokenOfOwnerByIndex calls tokenOfOwnerByIndex(address, uint256) on the NFT Position Manager.
func v3TokenOfOwnerByIndex(ctx context.Context, client *ethclient.Client, owner common.Address, index uint64) (*big.Int, error) {
	// selector: tokenOfOwnerByIndex(address,uint256) = 0x2f745c59
	data := make([]byte, 68)
	copy(data[:4], []byte{0x2f, 0x74, 0x5c, 0x59})
	copy(data[16:36], owner.Bytes()) // address in slot 0, right-aligned
	// index as uint256 in slot 1 (data[36:68]), right-aligned
	idxBytes := new(big.Int).SetUint64(index).Bytes()
	copy(data[68-len(idxBytes):68], idxBytes)

	result, err := client.CallContract(ctx, ethereum.CallMsg{To: &v3NftPositionManager, Data: data}, nil)
	if err != nil {
		return nil, err
	}
	if len(result) < 32 {
		return nil, fmt.Errorf("short response: %d bytes", len(result))
	}
	return new(big.Int).SetBytes(result[:32]), nil
}

// v3FetchPosition calls positions(uint256) on the NFT Position Manager and parses the result.
//
// Return layout (12 × 32-byte ABI slots):
//   0: uint96  nonce
//   1: address operator
//   2: address token0
//   3: address token1
//   4: uint24  fee
//   5: int24   tickLower
//   6: int24   tickUpper
//   7: uint128 liquidity
//   8: uint256 feeGrowthInside0LastX128
//   9: uint256 feeGrowthInside1LastX128
//  10: uint128 tokensOwed0
//  11: uint128 tokensOwed1
func v3FetchPosition(ctx context.Context, client *ethclient.Client, tokenId *big.Int) (LiquidityPosition, error) {
	var pos LiquidityPosition
	pos.TokenID = tokenId

	// selector: positions(uint256) = 0x99fbab88
	data := make([]byte, 36)
	copy(data[:4], []byte{0x99, 0xfb, 0xab, 0x88})
	idBytes := tokenId.Bytes()
	copy(data[36-len(idBytes):36], idBytes)

	result, err := client.CallContract(ctx, ethereum.CallMsg{To: &v3NftPositionManager, Data: data}, nil)
	if err != nil {
		return pos, err
	}
	if len(result) < 384 {
		return pos, fmt.Errorf("short positions response: %d bytes (expected 384)", len(result))
	}

	pos.Token0 = common.BytesToAddress(result[64:96])   // slot 2
	pos.Token1 = common.BytesToAddress(result[96:128])  // slot 3
	pos.Fee = uint32(new(big.Int).SetBytes(result[128:160]).Uint64()) // slot 4
	pos.TickLower = v3DecodeInt24(result[160:192])                    // slot 5
	pos.TickUpper = v3DecodeInt24(result[192:224])                    // slot 6
	pos.Liquidity = new(big.Int).SetBytes(result[224:256])            // slot 7
	pos.TokensOwed0 = new(big.Int).SetBytes(result[320:352])          // slot 10
	pos.TokensOwed1 = new(big.Int).SetBytes(result[352:384])          // slot 11

	return pos, nil
}

// v3DecodeInt24 decodes a sign-extended int24 from a 32-byte ABI slot.
func v3DecodeInt24(slot []byte) int32 {
	// The actual 3-byte value is right-aligned in the 32-byte slot.
	raw := uint32(slot[29])<<16 | uint32(slot[30])<<8 | uint32(slot[31])
	if raw&0x800000 != 0 {
		// Negative: sign extend to int32
		return int32(raw | 0xFF000000)
	}
	return int32(raw)
}

// v3ERC20Decimals fetches the decimals() of an ERC-20 token, returning 18 on failure.
func v3ERC20Decimals(ctx context.Context, client *ethclient.Client, addr common.Address) uint8 {
	// selector: decimals() = 0x313ce567
	callCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	result, err := client.CallContract(callCtx, ethereum.CallMsg{
		To:   &addr,
		Data: []byte{0x31, 0x3c, 0xe5, 0x67},
	}, nil)
	if err != nil || len(result) < 32 {
		return 18
	}
	return result[31]
}
