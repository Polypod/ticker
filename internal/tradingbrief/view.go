package tradingbrief

import (
	"fmt"
	"strings"

	c "github.com/achannarasappa/ticker/v5/internal/common"
)

// View renders the latest review-only analysis snapshot for the terminal UI.
func (b Brief) View(styles c.Styles) string {
	lines := []string{styles.TextBold("TRADING BRIEF · REVIEW ONLY")}
	if !b.GeneratedAt.IsZero() {
		lines[0] += styles.TextLight(" · " + b.GeneratedAt.Format("15:04:05"))
	}
	lines = append(lines,
		styles.TextLabel("Regime: ")+styles.Text(b.Regime),
		styles.TextLabel("New-risk gate: ")+styles.Text(b.NewRiskGate),
	)
	if len(b.CoveredSymbols) > 0 {
		lines = append(lines, styles.TextLabel("Covered symbols: ")+styles.Text(strings.Join(b.CoveredSymbols, ", ")))
	}
	if b.Account != nil {
		currency := b.Account.Currency
		accountStatus := b.Account.Status
		if b.Account.Stale {
			accountStatus = "STALE · " + accountStatus
		}
		lines = append(lines,
			styles.TextLabel("IBKR balances & positions: ")+styles.Text(accountStatus),
			styles.Text(fmt.Sprintf("Net liquidation: %.2f %s · Cash: %.2f · Settled cash: %.2f", b.Account.NetLiquidation, currency, b.Account.Cash, b.Account.SettledCash)),
			styles.Text(fmt.Sprintf("Buying power / available funds: %.2f / %.2f · Gross position value: %.2f · Unrealized P&L: %+.2f · Leverage: %.2fx", b.Account.BuyingPower, b.Account.AvailableFunds, b.Account.GrossPositionValue, b.Account.UnrealizedPnL, b.Account.Leverage)),
			styles.TextLabel("IBKR positions (streamed; no market-data snapshot):"),
		)
		for _, position := range b.Account.Positions {
			line := fmt.Sprintf("• %s %.2f shares · value %.2f · unrealized P&L %+.2f", position.Symbol, position.Quantity, position.MarketValue, position.UnrealizedPnL)
			if position.DailyPnL != 0 {
				line += fmt.Sprintf(" · day P&L %+.2f", position.DailyPnL)
			}
			lines = append(lines, styles.Text(line))
		}
	}
	if len(b.RiskFlags) > 0 {
		lines = append(lines, styles.TextLabel("Risk flags:"))
		for _, flag := range b.RiskFlags {
			lines = append(lines, styles.Text("• "+flag))
		}
	}
	lines = append(lines, styles.TextLabel("News sentiment: ")+styles.Text(b.NewsSentiment))
	for _, headline := range b.NewsHighlights {
		lines = append(lines, styles.TextLight("• "+headline))
	}
	if b.AISummary != "" {
		lines = append(lines, styles.TextLabel("AI review: ")+styles.Text(b.AISummary))
	}
	if len(b.ActionItems) > 0 {
		lines = append(lines, styles.TextLabel("Action items — human review required:"))
		for _, action := range b.ActionItems {
			lines = append(lines, styles.Text("• "+action))
		}
	}
	if b.Status != "" {
		lines = append(lines, styles.TextLight("Status: "+b.Status))
	}

	return strings.Join(lines, "\n")
}
