// Package server exposes quotes over a loopback HTTP API so GUI clients read
// the same data as the terminal UI. It is a second front end onto the existing
// monitor, not a replacement: `ticker` and `ticker print` are unaffected.
package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/adrg/xdg"

	"github.com/achannarasappa/ticker/v5/internal/asset"
	c "github.com/achannarasappa/ticker/v5/internal/common"
)

// Options configures the serve command.
type Options struct {
	Address string
	Token   string
}

// Snapshot is the full state sent on request and on every quote update. Clients
// replace their state with it wholesale rather than merging deltas.
type Snapshot struct {
	Assets    []c.Asset             `json:"assets"`
	Summary   asset.PositionSummary `json:"summary"`
	Sources   map[string]string     `json:"sources"`
	UpdatedAt time.Time             `json:"updatedAt"`
}

// quoteSourceNames maps the internal source enum to stable strings so clients
// can show per-row provenance without depending on iota ordering.
var quoteSourceNames = map[c.QuoteSource]string{ //nolint:gochecknoglobals
	c.QuoteSourceYahoo:       "yahoo",
	c.QuoteSourceUserDefined: "user",
	c.QuoteSourceCoingecko:   "coingecko",
	c.QuoteSourceUnknown:     "unknown",
	c.QuoteSourceCoinCap:     "coincap",
	c.QuoteSourceCoinbase:    "coinbase",
	c.QuoteSourceTiingo:      "tiingo",
}

func sourceName(source c.QuoteSource) string {
	if name, exists := quoteSourceNames[source]; exists {
		return name
	}

	return "unknown"
}

// NewSnapshot builds a client payload from an asset group quote.
func NewSnapshot(ctx c.Context, assetGroupQuote c.AssetGroupQuote) Snapshot {
	assets, summary := asset.GetAssets(ctx, assetGroupQuote)
	sources := make(map[string]string, len(assets))

	for _, a := range assets {
		sources[a.Symbol] = sourceName(a.QuoteSource)
	}

	return Snapshot{
		Assets:    assets,
		Summary:   summary,
		Sources:   sources,
		UpdatedAt: time.Now(),
	}
}

// Hub fans snapshots out to connected stream clients. Each client holds a
// single latest-wins slot, so a slow reader coalesces updates instead of
// blocking the monitor or growing an unbounded queue.
type Hub struct {
	mu      sync.Mutex
	clients map[chan []byte]struct{}
}

func newHub() *Hub {
	return &Hub{clients: make(map[chan []byte]struct{})}
}

func (h *Hub) add() chan []byte {
	channel := make(chan []byte, 1)

	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[channel] = struct{}{}

	return channel
}

func (h *Hub) remove(channel chan []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.clients, channel)
}

func (h *Hub) broadcast(payload []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()

	for channel := range h.clients {
		select {
		case <-channel: // discard the unread previous snapshot
		default:
		}

		select {
		case channel <- payload:
		default:
		}
	}
}

// NewHandler builds the HTTP API. snapshot is called for one-shot reads; the
// returned hub is how the caller pushes updates to stream clients.
func NewHandler(token string, snapshot func() Snapshot) (http.Handler, *Hub) {
	streams := newHub()
	mux := http.NewServeMux()

	mux.HandleFunc("GET /quotes", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// c.Asset is the shared domain type and carries no json tags, so fields
		// serialize under their Go names. Tagging it would churn every package
		// that marshals an asset; clients decode the Go names instead.
		//nolint:errcheck,musttag
		json.NewEncoder(w).Encode(snapshot())
	})

	mux.HandleFunc("GET /stream", func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)

		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)

			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		channel := streams.add()
		defer streams.remove(channel)

		writeEvent(w, flusher, mustMarshal(snapshot()))

		// Browsers reconnect an EventSource silently; the heartbeat is how a
		// client notices a connection that died without a close.
		heartbeat := time.NewTicker(25 * time.Second)
		defer heartbeat.Stop()

		for {
			select {
			case <-r.Context().Done():

				return
			case payload := <-channel:
				writeEvent(w, flusher, payload)
			case <-heartbeat.C:
				fmt.Fprint(w, ": ping\n\n")
				flusher.Flush()
			}
		}
	})

	return authenticate(token, mux), streams
}

func writeEvent(w http.ResponseWriter, flusher http.Flusher, payload []byte) {
	fmt.Fprintf(w, "event: quotes\ndata: %s\n\n", payload)
	flusher.Flush()
}

func mustMarshal(snapshot Snapshot) []byte {
	payload, err := json.Marshal(snapshot) //nolint:musttag

	if err != nil {
		return []byte("{}")
	}

	return payload
}

// authenticate requires the token on every request. EventSource cannot set
// headers, so the stream endpoint also accepts it as a query parameter.
func authenticate(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		provided := r.URL.Query().Get("token")

		if header := r.Header.Get("Authorization"); len(header) > 7 && header[:7] == "Bearer " {
			provided = header[7:]
		}

		if provided != token {
			http.Error(w, "unauthorized", http.StatusUnauthorized)

			return
		}

		next.ServeHTTP(w, r)
	})
}

// GenerateToken returns a random bearer token.
func GenerateToken() (string, error) {
	buffer := make([]byte, 32)

	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("unable to generate token: %w", err)
	}

	return hex.EncodeToString(buffer), nil
}

// discovery is how a GUI client finds a running daemon.
type discovery struct {
	URL   string `json:"url"`
	Token string `json:"token"`
	PID   int    `json:"pid"`
}

// DiscoveryPath is the file a client reads to find the daemon.
func DiscoveryPath() string {
	return filepath.Join(xdg.StateHome, "ticker", "serve.json")
}

// WriteDiscovery records the listening address and token for GUI clients.
func WriteDiscovery(listener net.Listener, token string) (string, error) {
	path := DiscoveryPath()

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", fmt.Errorf("unable to create state directory: %w", err)
	}

	payload, err := json.Marshal(discovery{
		URL:   "http://" + listener.Addr().String(),
		Token: token,
		PID:   os.Getpid(),
	})

	if err != nil {
		return "", fmt.Errorf("unable to encode discovery file: %w", err)
	}

	// The token is a credential for the whole portfolio: owner-readable only.
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		return "", fmt.Errorf("unable to write discovery file: %w", err)
	}

	return path, nil
}
