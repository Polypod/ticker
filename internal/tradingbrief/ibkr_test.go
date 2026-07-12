package tradingbrief

import (
	"testing"

	ibkr "github.com/osauer/ibkr/pkg/ibkr"
)

func TestNormalizeIBKRAccountUsesOnlyStreamedValues(t *testing.T) {
	t.Parallel()

	netLiq := 7945.01
	buyingPower := 179.01
	availableFunds := 179.01
	cash := 179.01
	grossValue := 7766.0
	unrealizedPnL := 1438.0
	snapshot := normalizeIBKRAccount(&ibkr.RawAccountSummary{
		AccountID:          "U1",
		AvailableFunds:     &availableFunds,
		BuyingPower:        &buyingPower,
		Currency:           "USD",
		GrossPositionValue: &grossValue,
		NetLiquidation:     &netLiq,
		TotalCashValue:     &cash,
		UnrealizedPnL:      &unrealizedPnL,
		Raw:                map[string]string{"SettledCash": "179.01"},
	}, []*ibkr.RawPosition{{
		AverageCost:   151.6,
		MarketPrice:   210.58,
		MarketValue:   4211.6,
		Position:      20,
		UnrealizedPNL: 1179.6,
		Contract:      ibkr.Contract{Symbol: "NVDA"},
	}})

	if snapshot.AccountID != "U1" || snapshot.Leverage != grossValue/netLiq || len(snapshot.Positions) != 1 {
		t.Fatalf("unexpected snapshot: %#v", snapshot)
	}
	if snapshot.Positions[0].DailyPnL != 0 {
		t.Fatalf("daily P&L must not be populated without a separate P&L stream: %#v", snapshot.Positions[0])
	}
}
