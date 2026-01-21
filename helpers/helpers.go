package helpers

import (
	"fmt"
	"image/color"
	"math/big"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/lucasb-eyer/go-colorful"
	"github.com/muesli/gamut"
)

// ShortenAddr shortens an Ethereum address for display
func ShortenAddr(addr string) string {
	if len(addr) < 10 {
		return addr
	}
	return addr[:6] + "…" + addr[len(addr)-4:]
}

// IsValidEthAddress checks if a string is a valid Ethereum address
func IsValidEthAddress(s string) bool {
	re := regexp.MustCompile("^0x[0-9a-fA-F]{40}$")
	return re.MatchString(s)
}

// FormatETH formats Wei to ETH with proper decimals
func FormatETH(wei *big.Int) string {
	if wei == nil {
		return "0 ETH"
	}
	eth := new(big.Float).Quo(new(big.Float).SetInt(wei), big.NewFloat(1e18))
	return eth.Text('f', 6) + " ETH"
}

// FormatToken formats token balance with proper decimals
func FormatToken(balance *big.Int, decimals uint8, symbol string) string {
	if balance == nil {
		return "0 " + symbol
	}
	divisor := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil))
	amount := new(big.Float).Quo(new(big.Float).SetInt(balance), divisor)
	return amount.Text('f', 4) + " " + symbol
}

// LoadedAt formats the loaded timestamp
func LoadedAt(t time.Time, loading bool) string {
	if loading {
		return "loading…"
	}
	if t.IsZero() {
		return "never"
	}
	return t.Format("15:04:05")
}

// FadeString creates a gradient colored string
func FadeString(s string, firstColor string, lastColor string) string {
	blends := gamut.Blends(lipgloss.Color(firstColor), lipgloss.Color(lastColor), len(s))
	return rainbow(lipgloss.NewStyle(), s, blends)
}

func rainbow(baseStyle lipgloss.Style, str string, colors []color.Color) string {
	var result string
	for i, c := range str {
		col, _ := colorful.MakeColor(colors[i%len(colors)])
		result += baseStyle.Foreground(lipgloss.Color(col.Hex())).Render(string(c))
	}
	return result
}

// Max returns the maximum of two integers
func Max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// Contains checks if a string slice contains a value
func Contains(slice []string, val string) bool {
	for _, item := range slice {
		if strings.EqualFold(item, val) {
			return true
		}
	}
	return false
}

// ToHex converts a color to hex string
func ToHex(c color.Color) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("#%02X%02X%02X", r>>8, g>>8, b>>8)
}
