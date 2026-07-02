// Command updateondotokens refreshes helpers/data/ondo_gm_tokens.json from
// Ondo Finance's official, versioned token list
// (github.com/ondoprotocol/ondo-global-markets-token-list). It is a
// maintainer-run dev tool, never invoked by the shipped TUI binary — see the
// "Dev tooling" note in CLAUDE.md for why this is exempt from the app's
// no-external-HTTP constraint.
//
// Usage: go run ./cmd/updateondotokens
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"sort"
	"time"
)

const sourceURL = "https://raw.githubusercontent.com/ondoprotocol/ondo-global-markets-token-list/main/tokenlist.json"

const outPath = "helpers/data/ondo_gm_tokens.json"

// excludeSymbols lists tokens already hardcoded elsewhere in the app
// (helpers.UniswapNetworkAddresses.SPCXon), so they aren't duplicated here.
var excludeSymbols = map[string]bool{
	"SPCXon": true,
}

var hexAddrRe = regexp.MustCompile(`^0x[0-9a-fA-F]{40}$`)

type upstreamList struct {
	Name      string `json:"name"`
	Timestamp string `json:"timestamp"`
	Version   struct {
		Major int `json:"major"`
		Minor int `json:"minor"`
		Patch int `json:"patch"`
	} `json:"version"`
	Tokens []upstreamToken `json:"tokens"`
}

type upstreamToken struct {
	ChainID  int    `json:"chainId"`
	Address  string `json:"address"`
	Name     string `json:"name"`
	Symbol   string `json:"symbol"`
	Decimals int    `json:"decimals"`
}

type vendoredToken struct {
	Symbol   string `json:"symbol"`
	Name     string `json:"name"`
	Address  string `json:"address"`
	Decimals int    `json:"decimals"`
}

type vendoredList struct {
	SourceName      string          `json:"source_name"`
	SourceTimestamp string          `json:"source_timestamp"`
	SourceVersion   string          `json:"source_version"`
	FetchedAt       string          `json:"fetched_at"`
	Tokens          []vendoredToken `json:"tokens"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "updateondotokens:", err)
		os.Exit(1)
	}
}

func run() error {
	req, err := http.NewRequest(http.MethodGet, sourceURL, nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("fetch %s: %w", sourceURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch %s: status %d", sourceURL, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var upstream upstreamList
	if err := json.Unmarshal(body, &upstream); err != nil {
		return fmt.Errorf("parse upstream token list: %w", err)
	}

	seen := make(map[string]bool, len(upstream.Tokens))
	var out []vendoredToken
	for _, t := range upstream.Tokens {
		if t.ChainID != 1 {
			continue
		}
		if excludeSymbols[t.Symbol] {
			continue
		}
		if !hexAddrRe.MatchString(t.Address) {
			fmt.Fprintf(os.Stderr, "updateondotokens: skipping %s — invalid address %q\n", t.Symbol, t.Address)
			continue
		}
		key := t.Address
		for i := range key {
			// lowercase manually to avoid pulling in strings.ToLower just for this
			if key[i] >= 'A' && key[i] <= 'Z' {
				b := []byte(key)
				b[i] += 'a' - 'A'
				key = string(b)
			}
		}
		if seen[key] {
			fmt.Fprintf(os.Stderr, "updateondotokens: skipping %s — duplicate address %s\n", t.Symbol, t.Address)
			continue
		}
		seen[key] = true
		out = append(out, vendoredToken{
			Symbol:   t.Symbol,
			Name:     t.Name,
			Address:  t.Address,
			Decimals: t.Decimals,
		})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Symbol < out[j].Symbol })

	vendored := vendoredList{
		SourceName:      upstream.Name,
		SourceTimestamp: upstream.Timestamp,
		SourceVersion:   fmt.Sprintf("%d.%d.%d", upstream.Version.Major, upstream.Version.Minor, upstream.Version.Patch),
		FetchedAt:       time.Now().UTC().Format(time.RFC3339),
		Tokens:          out,
	}

	data, err := json.MarshalIndent(vendored, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(outPath, data, 0644); err != nil {
		return err
	}
	fmt.Printf("updateondotokens: wrote %d mainnet tokens to %s (source v%s)\n", len(out), outPath, vendored.SourceVersion)
	return nil
}
