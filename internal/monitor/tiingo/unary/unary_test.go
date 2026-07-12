package unary

import (
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetAssetQuotes(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/iex/AAPL,MSFT" {
			t.Errorf("request path = %q", r.URL.Path)
		}
		if authorization := r.Header.Get("Authorization"); authorization != "Token test-token" {
			t.Errorf("authorization = %q", authorization)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"ticker":"AAPL","tngoLast":200,"prevClose":195,"open":196,"high":201,"low":194,"volume":12345},
			{"ticker":"MSFT","tngoLast":400,"prevClose":400,"open":399,"high":402,"low":398,"volume":54321}
		]`))
	}))
	defer server.Close()

	api := NewUnaryAPI(Config{BaseURL: server.URL, Token: "test-token"})
	quotes, err := api.GetAssetQuotes([]string{"AAPL", "MSFT"})
	if err != nil {
		t.Fatalf("GetAssetQuotes() error = %v", err)
	}
	if len(quotes) != 2 {
		t.Fatalf("quote count = %d, want 2", len(quotes))
	}

	quote := quotes[0]
	if quote.Symbol != "AAPL.TI" || quote.Meta.SymbolInSourceAPI != "AAPL" {
		t.Fatalf("unexpected symbol mapping: %#v", quote)
	}
	if quote.QuotePrice.Change != 5 || math.Abs(quote.QuotePrice.ChangePercent-5.0/195.0*100) > 1e-12 {
		t.Fatalf("unexpected price mapping: %#v", quote.QuotePrice)
	}
	if quote.Currency.FromCurrencyCode != "USD" || quote.QuoteExtended.Volume != 12345 {
		t.Fatalf("unexpected quote metadata: %#v", quote)
	}
}

func TestGetAssetQuotesRequiresToken(t *testing.T) {
	t.Parallel()

	api := NewUnaryAPI(Config{BaseURL: "https://api.tiingo.com"})
	_, err := api.GetAssetQuotes([]string{"AAPL"})
	if err == nil || err.Error() != "get Tiingo IEX quotes: TIINGO_API_TOKEN is required for .TI symbols" {
		t.Fatalf("GetAssetQuotes() error = %v", err)
	}
}

func TestGetReferenceData(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if authorization := r.Header.Get("Authorization"); authorization != "Token test-token" {
			t.Errorf("authorization = %q", authorization)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/tiingo/daily/AAPL/prices":
			_, _ = w.Write([]byte(`[{"high":190,"low":160},{"high":210,"low":140}]`))
		case "/tiingo/fundamentals/AAPL/daily":
			_, _ = w.Write([]byte(`[{"marketCap":2900000000000},{"marketCap":3000000000000}]`))
		default:
			t.Errorf("request path = %q", r.URL.Path)
		}
	}))
	defer server.Close()

	api := NewUnaryAPI(Config{BaseURL: server.URL, Token: "test-token"})
	rangeData, err := api.GetFiftyTwoWeekRange("AAPL")
	if err != nil {
		t.Fatalf("GetFiftyTwoWeekRange() error = %v", err)
	}
	if !rangeData.HasFiftyTwoWeekRange || rangeData.FiftyTwoWeekLow != 140 || rangeData.FiftyTwoWeekHigh != 210 {
		t.Fatalf("unexpected range data: %#v", rangeData)
	}

	marketCapData, err := api.GetMarketCap("AAPL")
	if err != nil {
		t.Fatalf("GetMarketCap() error = %v", err)
	}
	if !marketCapData.HasMarketCap || marketCapData.MarketCap != 3000000000000 {
		t.Fatalf("unexpected market cap data: %#v", marketCapData)
	}
}
