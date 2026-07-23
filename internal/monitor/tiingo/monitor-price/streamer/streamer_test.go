package streamer

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	c "github.com/achannarasappa/ticker/v5/internal/common"
	"github.com/gorilla/websocket"
)

func TestParseReferencePriceUpdates(t *testing.T) {
	t.Parallel()

	data := json.RawMessage(`["2026-07-11T13:30:00Z","AAPL",200.25]`)
	updates, err := parseReferencePriceUpdates(message{MessageType: "A", Data: data})
	if err != nil {
		t.Fatalf("parseReferencePriceUpdates() error = %v", err)
	}
	want := []QuoteUpdate{{Symbol: "AAPL", Price: 200.25}}
	if !reflect.DeepEqual(updates, want) {
		t.Fatalf("updates = %#v, want %#v", updates, want)
	}
}

func TestParseReferencePriceUpdatesNormalizesTickerCase(t *testing.T) {
	t.Parallel()

	// Live Tiingo IEX websocket messages use lowercase tickers.
	data := json.RawMessage(`["2026-07-11T13:30:00Z","nvda",208.47]`)
	updates, err := parseReferencePriceUpdates(message{MessageType: "A", Data: data})
	if err != nil {
		t.Fatalf("parseReferencePriceUpdates() error = %v", err)
	}
	want := []QuoteUpdate{{Symbol: "NVDA", Price: 208.47}}
	if !reflect.DeepEqual(updates, want) {
		t.Fatalf("updates = %#v, want %#v", updates, want)
	}
}

func TestParseReferencePriceUpdatesBatch(t *testing.T) {
	t.Parallel()

	data := json.RawMessage(`[["2026-07-11T13:30:00Z","AAPL",200.25],["2026-07-11T13:30:00Z","MSFT",450.5]]`)
	updates, err := parseReferencePriceUpdates(message{MessageType: "A", Data: data})
	if err != nil {
		t.Fatalf("parseReferencePriceUpdates() error = %v", err)
	}
	want := []QuoteUpdate{{Symbol: "AAPL", Price: 200.25}, {Symbol: "MSFT", Price: 450.5}}
	if !reflect.DeepEqual(updates, want) {
		t.Fatalf("updates = %#v, want %#v", updates, want)
	}
}

func TestSubscriptionUsesEntitlementSafeDefaults(t *testing.T) {
	t.Parallel()

	stream := NewStreamer(context.Background(), Config{Token: "test-token", ThresholdLevel: 6})
	subscription := stream.newSubscription([]string{"AAPL"})
	if subscription.EventName != "subscribe" || subscription.EventData.ThresholdLevel != 6 {
		t.Fatalf("unexpected subscription: %#v", subscription)
	}
	if subscription.EventData.AuthToken != "test-token" || subscription.Authorization != "test-token" {
		t.Fatalf("missing subscription token: %#v", subscription)
	}
}

func TestStreamerSubscribesAndEmitsReferencePrice(t *testing.T) {
	t.Parallel()

	upgrader := websocket.Upgrader{CheckOrigin: func(_ *http.Request) bool { return true }}
	subscriptions := make(chan subscription, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade websocket: %v", err)

			return
		}
		defer conn.Close()

		var subscription subscription
		if err := conn.ReadJSON(&subscription); err != nil {
			t.Errorf("read subscription: %v", err)

			return
		}
		subscriptions <- subscription
		if err := conn.WriteJSON(message{MessageType: "A", Data: json.RawMessage(`["2026-07-11T13:30:00Z","AAPL",200.25]`)}); err != nil {
			t.Errorf("write price update: %v", err)
		}
	}))
	defer server.Close()

	updates := make(chan c.MessageUpdate[QuoteUpdate], 1)
	stream := NewStreamer(context.Background(), Config{
		Token:          "test-token",
		URL:            "ws://" + server.URL[len("http://"):],
		ThresholdLevel: 6,
		ChanError:      make(chan error, 1),
		ChanUpdate:     updates,
	})
	stream.SetSymbols([]string{"AAPL"}, 7)
	if err := stream.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer stream.Stop()

	select {
	case subscription := <-subscriptions:
		if !reflect.DeepEqual(subscription.EventData.Tickers, []string{"AAPL"}) || subscription.EventData.ThresholdLevel != 6 {
			t.Fatalf("unexpected subscription: %#v", subscription)
		}
	case <-time.After(time.Second):
		t.Fatal("did not receive subscription")
	}

	select {
	case update := <-updates:
		if update.ID != "AAPL" || update.Data.Price != 200.25 || update.VersionVector != 7 {
			t.Fatalf("unexpected price update: %#v", update)
		}
	case <-time.After(time.Second):
		t.Fatal("did not receive price update")
	}
}
