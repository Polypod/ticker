package monitorPriceTiingo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/achannarasappa/ticker/v5/internal/cache"
	c "github.com/achannarasappa/ticker/v5/internal/common"
	"github.com/spf13/afero"
)

func TestSetSymbolsRefreshesTiingoSnapshot(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if authorization := r.Header.Get("Authorization"); authorization != "Token test-token" {
			t.Errorf("authorization = %q", authorization)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/iex/AAPL":
			_, _ = w.Write([]byte(`[{"ticker":"AAPL","tngoLast":200,"prevClose":195,"open":196,"high":201,"low":194,"volume":12345}]`))
		case "/tiingo/daily/AAPL/prices":
			_, _ = w.Write([]byte(`[{"high":210,"low":150},{"high":220,"low":140}]`))
		case "/tiingo/fundamentals/AAPL/daily":
			_, _ = w.Write([]byte(`[{"marketCap":3000000000000}]`))
		default:
			t.Errorf("unexpected request path = %q", r.URL.Path)
		}
	}))
	defer server.Close()

	monitor := NewMonitorPriceTiingo(Config{
		BaseURL:                  server.URL,
		Ctx:                      context.Background(),
		Token:                    "test-token",
		StreamingURL:             "ws://example.test/iex",
		ChanError:                make(chan error, 1),
		ChanRequestCurrencyRates: make(chan []string, 1),
		ChanUpdateAssetQuote:     make(chan c.MessageUpdate[c.AssetQuote], 1),
	})
	if err := monitor.SetSymbols([]string{"AAPL"}, 3); err != nil {
		t.Fatalf("SetSymbols() error = %v", err)
	}

	quotes, err := monitor.GetAssetQuotes()
	if err != nil {
		t.Fatalf("GetAssetQuotes() error = %v", err)
	}
	if len(quotes) != 1 || quotes[0].Symbol != "AAPL.TI" || quotes[0].QuotePrice.Price != 200 {
		t.Fatalf("unexpected quotes: %#v", quotes)
	}
	if quotes[0].QuoteExtended.FiftyTwoWeekLow != 140 || quotes[0].QuoteExtended.FiftyTwoWeekHigh != 220 || quotes[0].QuoteExtended.MarketCap != 3000000000000 {
		t.Fatalf("reference data was not applied: %#v", quotes[0].QuoteExtended)
	}

	if err := monitor.SetCurrencyRates(c.CurrencyRates{"USD": {FromCurrency: "USD", ToCurrency: "EUR", Rate: 0.9}}); err != nil {
		t.Fatalf("SetCurrencyRates() error = %v", err)
	}
	quotes, err = monitor.GetAssetQuotes()
	if err != nil {
		t.Fatalf("GetAssetQuotes() error = %v", err)
	}
	if quotes[0].Currency.Rate != 0.9 || quotes[0].Currency.ToCurrencyCode != "EUR" {
		t.Fatalf("currency rate was not applied: %#v", quotes[0].Currency)
	}
}

func TestSetSymbolsKeepsQuotesWhenFundamentalsAreUnavailable(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/iex/AAPL":
			_, _ = w.Write([]byte(`[{"ticker":"AAPL","tngoLast":200,"prevClose":195,"open":196,"high":201,"low":194,"volume":12345}]`))
		case "/tiingo/daily/AAPL/prices":
			_, _ = w.Write([]byte(`[{"high":210,"low":140}]`))
		case "/tiingo/fundamentals/AAPL/daily":
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"detail":"not entitled"}`))
		default:
			t.Errorf("unexpected request path = %q", r.URL.Path)
		}
	}))
	defer server.Close()

	monitor := NewMonitorPriceTiingo(Config{
		BaseURL:                  server.URL,
		Ctx:                      context.Background(),
		Token:                    "test-token",
		StreamingURL:             "ws://example.test/iex",
		ChanError:                make(chan error, 1),
		ChanRequestCurrencyRates: make(chan []string, 1),
		ChanUpdateAssetQuote:     make(chan c.MessageUpdate[c.AssetQuote], 1),
	})
	if err := monitor.SetSymbols([]string{"AAPL"}, 3); err != nil {
		t.Fatalf("SetSymbols() error = %v", err)
	}

	quotes, err := monitor.GetAssetQuotes()
	if err != nil {
		t.Fatalf("GetAssetQuotes() error = %v", err)
	}
	if quotes[0].QuoteExtended.FiftyTwoWeekLow != 140 || quotes[0].QuoteExtended.FiftyTwoWeekHigh != 210 || quotes[0].QuoteExtended.MarketCap != 0 {
		t.Fatalf("unexpected enriched quote: %#v", quotes[0].QuoteExtended)
	}
}

func TestReferenceDataIsSharedThroughTheStartupCache(t *testing.T) {
	t.Parallel()

	var eodRequests, fundamentalsRequests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/iex/AAPL":
			_, _ = w.Write([]byte(`[{"ticker":"AAPL","tngoLast":200,"prevClose":195,"open":196,"high":201,"low":194,"volume":12345}]`))
		case "/tiingo/daily/AAPL/prices":
			eodRequests++
			_, _ = w.Write([]byte(`[{"high":210,"low":140}]`))
		case "/tiingo/fundamentals/AAPL/daily":
			fundamentalsRequests++
			_, _ = w.Write([]byte(`[{"marketCap":3000000000000}]`))
		default:
			t.Errorf("unexpected request path = %q", r.URL.Path)
		}
	}))
	defer server.Close()

	startupCache := cache.New(afero.NewMemMapFs(), "/ticker/cache.json", true)
	newMonitor := func() *MonitorPriceTiingo {
		return NewMonitorPriceTiingo(Config{
			BaseURL:                  server.URL,
			Cache:                    startupCache,
			Ctx:                      context.Background(),
			Token:                    "test-token",
			StreamingURL:             "ws://example.test/iex",
			ChanError:                make(chan error, 1),
			ChanRequestCurrencyRates: make(chan []string, 1),
			ChanUpdateAssetQuote:     make(chan c.MessageUpdate[c.AssetQuote], 1),
		})
	}

	if err := newMonitor().SetSymbols([]string{"AAPL"}, 1); err != nil {
		t.Fatalf("first SetSymbols() error = %v", err)
	}
	if err := newMonitor().SetSymbols([]string{"AAPL"}, 1); err != nil {
		t.Fatalf("second SetSymbols() error = %v", err)
	}
	if eodRequests != 1 || fundamentalsRequests != 1 {
		t.Fatalf("reference requests = EOD %d, fundamentals %d; want 1 each", eodRequests, fundamentalsRequests)
	}
}
