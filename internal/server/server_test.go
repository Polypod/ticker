package server

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	c "github.com/achannarasappa/ticker/v5/internal/common"
)

func fixtureSnapshot(price float64) Snapshot {
	return Snapshot{
		Assets: []c.Asset{{
			Symbol:      "NVDA",
			QuotePrice:  c.QuotePrice{Price: price},
			QuoteSource: c.QuoteSourceTiingo,
		}},
		Sources: map[string]string{"NVDA": "tiingo"},
	}
}

func TestQuotesRequiresToken(t *testing.T) {
	handler, _ := NewHandler("secret", func() Snapshot { return fixtureSnapshot(1) })

	for _, request := range []*http.Request{
		httptest.NewRequest(http.MethodGet, "/quotes", nil),
		httptest.NewRequest(http.MethodGet, "/quotes?token=wrong", nil),
	} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 for %s, got %d", request.URL, recorder.Code)
		}
	}
}

func TestQuotesReturnsSnapshot(t *testing.T) {
	handler, _ := NewHandler("secret", func() Snapshot { return fixtureSnapshot(123.45) })

	request := httptest.NewRequest(http.MethodGet, "/quotes", nil)
	request.Header.Set("Authorization", "Bearer secret")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}

	var snapshot Snapshot
	if err := json.Unmarshal(recorder.Body.Bytes(), &snapshot); err != nil {
		t.Fatalf("unable to decode snapshot: %v", err)
	}

	if len(snapshot.Assets) != 1 || snapshot.Assets[0].QuotePrice.Price != 123.45 {
		t.Fatalf("unexpected assets: %+v", snapshot.Assets)
	}

	if snapshot.Sources["NVDA"] != "tiingo" {
		t.Fatalf("expected tiingo provenance, got %q", snapshot.Sources["NVDA"])
	}
}

// The stream is the whole point of the daemon: a client must get the current
// state on connect and every later update without polling.
func TestStreamPushesUpdates(t *testing.T) {
	price := 1.0
	handler, streams := NewHandler("secret", func() Snapshot { return fixtureSnapshot(price) })

	server := httptest.NewServer(handler)
	defer server.Close()

	// EventSource cannot set headers, so the query parameter has to work.
	response, err := http.Get(server.URL + "/stream?token=secret") //nolint:noctx
	if err != nil {
		t.Fatalf("unable to open stream: %v", err)
	}
	defer response.Body.Close()

	reader := bufio.NewReader(response.Body)

	if got := readEvent(t, reader); !strings.Contains(got, `"Price":1`) {
		t.Fatalf("expected initial snapshot, got %q", got)
	}

	price = 2.0
	streams.broadcast(mustMarshal(fixtureSnapshot(price)))

	if got := readEvent(t, reader); !strings.Contains(got, `"Price":2`) {
		t.Fatalf("expected pushed update, got %q", got)
	}
}

// readEvent returns the data line of the next SSE event.
func readEvent(t *testing.T, reader *bufio.Reader) string {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)

	for time.Now().Before(deadline) {
		line, err := reader.ReadString('\n')

		if err != nil {
			t.Fatalf("unable to read stream: %v", err)
		}

		if strings.HasPrefix(line, "data: ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "data: "))
		}
	}

	t.Fatal("timed out waiting for an event")

	return ""
}
