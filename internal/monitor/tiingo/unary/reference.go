package unary

import (
	"fmt"
	"net/url"
	"time"
)

// ReferenceData is the slow-changing quote metadata that ticker enriches from
// Tiingo's EOD and Fundamentals APIs.
type ReferenceData struct {
	FiftyTwoWeekHigh     float64
	FiftyTwoWeekLow      float64
	HasFiftyTwoWeekRange bool
	MarketCap            float64
	HasMarketCap         bool
}

type eodPrice struct {
	High float64 `json:"high"`
	Low  float64 `json:"low"`
}

type dailyFundamental struct {
	MarketCap *float64 `json:"marketCap"`
}

// GetFiftyTwoWeekRange calculates a trailing 52-week range from Tiingo EOD prices.
func (u *UnaryAPI) GetFiftyTwoWeekRange(symbol string) (ReferenceData, error) {
	endDate := time.Now().UTC().Format(time.DateOnly)
	startDate := time.Now().UTC().AddDate(-1, 0, 0).Format(time.DateOnly)
	query := url.Values{}
	query.Set("startDate", startDate)
	query.Set("endDate", endDate)

	var prices []eodPrice
	if err := u.getJSON("/tiingo/daily/"+symbol+"/prices?"+query.Encode(), &prices); err != nil {
		return ReferenceData{}, fmt.Errorf("get Tiingo EOD prices for %s: %w", symbol, err)
	}
	if len(prices) == 0 {
		return ReferenceData{}, fmt.Errorf("tiingo returned no EOD prices for %s", symbol)
	}

	rangeData := ReferenceData{FiftyTwoWeekLow: prices[0].Low, FiftyTwoWeekHigh: prices[0].High}
	for _, price := range prices[1:] {
		if price.Low < rangeData.FiftyTwoWeekLow {
			rangeData.FiftyTwoWeekLow = price.Low
		}
		if price.High > rangeData.FiftyTwoWeekHigh {
			rangeData.FiftyTwoWeekHigh = price.High
		}
	}
	rangeData.HasFiftyTwoWeekRange = true

	return rangeData, nil
}

// GetMarketCap retrieves the latest daily market-cap metric from Tiingo Fundamentals.
func (u *UnaryAPI) GetMarketCap(symbol string) (ReferenceData, error) {
	query := url.Values{}
	query.Set("columns", "marketCap")

	var fundamentals []dailyFundamental
	if err := u.getJSON("/tiingo/fundamentals/"+symbol+"/daily?"+query.Encode(), &fundamentals); err != nil {
		return ReferenceData{}, fmt.Errorf("get Tiingo market cap for %s: %w", symbol, err)
	}

	for index := len(fundamentals) - 1; index >= 0; index-- {
		if fundamentals[index].MarketCap != nil {
			return ReferenceData{MarketCap: *fundamentals[index].MarketCap, HasMarketCap: true}, nil
		}
	}

	return ReferenceData{}, nil
}
