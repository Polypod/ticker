package tradingbrief

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	c "github.com/achannarasappa/ticker/v5/internal/common"
)

func TestAnalyzeBuildsReviewOnlyBrief(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Token tiingo-token" {
			t.Errorf("unexpected authorization header")
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/iex/SPY,QQQ,IWM":
			_, _ = w.Write([]byte(`[
				{"ticker":"SPY","tngoLast":101,"prevClose":100},
				{"ticker":"QQQ","tngoLast":101,"prevClose":100},
				{"ticker":"IWM","tngoLast":101,"prevClose":100}
			]`))
		case "/tiingo/news":
			if r.URL.Query().Get("tickers") != "AAPL,MSFT" {
				t.Errorf("tickers = %q, want AAPL,MSFT", r.URL.Query().Get("tickers"))
			}
			_, _ = w.Write([]byte(`[
				{"title":"Company reports growth","description":"Strong growth outlook","source":"example.com","tickers":["AAPL"]},
				{"title":"Unrelated story","description":"Not relevant","source":"example.com","tickers":["TSLA"]}
			]`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	brief, err := NewService(Config{
		TiingoBaseURL: server.URL,
		TiingoToken:   "tiingo-token",
	}).Analyze([]c.Asset{
		{Symbol: "AAPL.TI", Position: c.Position{Value: 1000}},
		{Symbol: "MSFT.TI", Position: c.Position{Value: 500}},
	})
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if brief.Regime != "Risk-on across SPY, QQQ, and IWM" || brief.NewRiskGate != "Selective new risk permitted" {
		t.Fatalf("unexpected regime: %#v", brief)
	}
	if brief.NewsSentiment == "No recent Tiingo news" || len(brief.NewsHighlights) != 1 {
		t.Fatalf("unexpected news summary: %#v", brief)
	}
	if len(brief.ActionItems) == 0 || brief.AISummary != "" {
		t.Fatalf("unexpected deterministic review: %#v", brief)
	}
	if strings.Join(brief.CoveredSymbols, ",") != "AAPL.TI,MSFT.TI" {
		t.Fatalf("covered symbols = %#v", brief.CoveredSymbols)
	}
}

func TestAnalyzeReportsOpenAIStatusWithoutLeakingResponseBody(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/iex/SPY,QQQ,IWM", "/tiingo/news":
			_, _ = w.Write([]byte(`[]`))
		case "/responses":
			http.Error(w, "sensitive upstream response", http.StatusTooManyRequests)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	brief, err := NewService(Config{
		OpenAIAPIKey:  "openai-token",
		OpenAIBaseURL: server.URL,
		TiingoBaseURL: server.URL,
		TiingoToken:   "tiingo-token",
	}).Analyze([]c.Asset{{Symbol: "AAPL.TI"}})
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if brief.Status != "AI synthesis unavailable (OpenAI HTTP 429)" {
		t.Fatalf("status = %q", brief.Status)
	}
}

func TestAnalyzeUsesSchemaConstrainedAIReview(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/iex/SPY,QQQ,IWM":
			_, _ = w.Write([]byte(`[]`))
		case "/tiingo/news":
			_, _ = w.Write([]byte(`[]`))
		case "/responses":
			if r.Header.Get("Authorization") != "Bearer openai-token" {
				t.Errorf("unexpected OpenAI authorization header")
			}
			var request map[string]any
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			if request["store"] != false {
				t.Errorf("store = %#v, want false", request["store"])
			}
			_, _ = w.Write([]byte(`{"output":[{"content":[{"text":"{\"summary\":\"Review only\",\"risk_flags\":[\"Risk flag\"],\"action_items\":[\"Check thesis\"]}"}]}]}`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	brief, err := NewService(Config{
		OpenAIAPIKey:  "openai-token",
		OpenAIBaseURL: server.URL,
		TiingoBaseURL: server.URL,
		TiingoToken:   "tiingo-token",
	}).Analyze([]c.Asset{{Symbol: "AAPL.TI"}})
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if brief.AISummary != "Review only" || len(brief.RiskFlags) != 1 || len(brief.ActionItems) != 1 {
		t.Fatalf("unexpected AI review: %#v", brief)
	}
}
