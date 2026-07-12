// Package unary retrieves current IEX quote snapshots from Tiingo.
package unary

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	c "github.com/achannarasappa/ticker/v5/internal/common"
)

// Config configures a Tiingo IEX REST client.
type Config struct {
	BaseURL string
	Token   string
}

// ResponseQuote is Tiingo's current IEX quote response.
type ResponseQuote struct {
	Ticker    string  `json:"ticker"`
	TngoLast  float64 `json:"tngoLast"`
	PrevClose float64 `json:"prevClose"`
	Open      float64 `json:"open"`
	High      float64 `json:"high"`
	Low       float64 `json:"low"`
	Volume    float64 `json:"volume"`
}

// UnaryAPI retrieves Tiingo IEX REST snapshots.
type UnaryAPI struct {
	baseURL string
	client  *http.Client
	token   string
}

// NewUnaryAPI creates a Tiingo IEX REST client.
func NewUnaryAPI(config Config) *UnaryAPI {
	return &UnaryAPI{
		baseURL: strings.TrimRight(config.BaseURL, "/"),
		client:  &http.Client{},
		token:   config.Token,
	}
}

// GetAssetQuotes retrieves a single REST snapshot for all requested symbols.
func (u *UnaryAPI) GetAssetQuotes(symbols []string) ([]c.AssetQuote, error) {
	if len(symbols) == 0 {
		return []c.AssetQuote{}, nil
	}

	var responseQuotes []ResponseQuote
	if err := u.getJSON("/iex/"+strings.Join(symbols, ","), &responseQuotes); err != nil {
		return nil, fmt.Errorf("get Tiingo IEX quotes: %w", err)
	}

	quotes := make([]c.AssetQuote, 0, len(responseQuotes))
	for _, responseQuote := range responseQuotes {
		quotes = append(quotes, transformResponseQuote(responseQuote))
	}

	return quotes, nil
}

func (u *UnaryAPI) getJSON(endpoint string, out any) error {
	if u.token == "" {
		return errors.New("TIINGO_API_TOKEN is required for .TI symbols")
	}

	requestURL, err := url.Parse(u.baseURL + endpoint)
	if err != nil {
		return fmt.Errorf("create Tiingo request: %w", err)
	}

	req, err := http.NewRequest(http.MethodGet, requestURL.String(), nil)
	if err != nil {
		return fmt.Errorf("create Tiingo request: %w", err)
	}
	req.Header.Set("Authorization", "Token "+u.token)

	resp, err := u.client.Do(req)
	if err != nil {
		return fmt.Errorf("request Tiingo endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("tiingo request failed with status %d", resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode Tiingo response: %w", err)
	}

	return nil
}

func transformResponseQuote(responseQuote ResponseQuote) c.AssetQuote {
	change := responseQuote.TngoLast - responseQuote.PrevClose
	changePercent := 0.0
	if responseQuote.PrevClose != 0 {
		changePercent = change / responseQuote.PrevClose * 100
	}

	return c.AssetQuote{
		Name:   responseQuote.Ticker,
		Symbol: responseQuote.Ticker + ".TI",
		Class:  c.AssetClassStock,
		Currency: c.Currency{
			FromCurrencyCode: "USD",
		},
		QuotePrice: c.QuotePrice{
			Price:          responseQuote.TngoLast,
			PricePrevClose: responseQuote.PrevClose,
			PriceOpen:      responseQuote.Open,
			PriceDayHigh:   responseQuote.High,
			PriceDayLow:    responseQuote.Low,
			Change:         change,
			ChangePercent:  changePercent,
		},
		QuoteExtended: c.QuoteExtended{Volume: responseQuote.Volume},
		QuoteSource:   c.QuoteSourceTiingo,
		Exchange: c.Exchange{
			Name:                    "Tiingo IEX",
			State:                   c.ExchangeStateOpen,
			IsActive:                responseQuote.TngoLast != 0,
			IsRegularTradingSession: true,
		},
		Meta: c.Meta{SymbolInSourceAPI: responseQuote.Ticker},
	}
}
