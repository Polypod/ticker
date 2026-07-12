package cli

import (
	"testing"

	c "github.com/achannarasappa/ticker/v5/internal/common"
)

func TestGetSymbolAndSourceTiingo(t *testing.T) {
	t.Parallel()

	result := getSymbolAndSource("aapl.ti", nil)
	if result.source != c.QuoteSourceTiingo || result.symbol != "AAPL" {
		t.Fatalf("getSymbolAndSource() = %#v, want Tiingo AAPL", result)
	}
}
