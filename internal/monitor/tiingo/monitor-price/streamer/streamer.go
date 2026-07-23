// Package streamer maintains the Tiingo IEX reference-price subscription.
package streamer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	c "github.com/achannarasappa/ticker/v5/internal/common"
	"github.com/gorilla/websocket"
)

const reconnectDelay = time.Second

type subscription struct {
	EventName     string           `json:"eventName"`
	Authorization string           `json:"authorization"`
	EventData     subscriptionData `json:"eventData"`
}

type subscriptionData struct {
	AuthToken      string   `json:"authToken"`
	Tickers        []string `json:"tickers"`
	ThresholdLevel int      `json:"thresholdLevel"`
}

type message struct {
	MessageType string          `json:"messageType"`
	Data        json.RawMessage `json:"data"`
}

// QuoteUpdate is a reference-price update received from Tiingo.
type QuoteUpdate struct {
	Symbol string
	Price  float64
}

// Config configures a Tiingo IEX websocket stream.
type Config struct {
	Token          string
	URL            string
	ThresholdLevel int
	ChanUpdate     chan c.MessageUpdate[QuoteUpdate]
	ChanError      chan error
}

// Streamer manages the websocket connection and reconnects whenever symbols change.
type Streamer struct {
	chanError  chan error
	chanUpdate chan c.MessageUpdate[QuoteUpdate]
	cancel     context.CancelFunc
	ctx        context.Context
	mu         sync.Mutex
	conn       *websocket.Conn
	isStarted  bool
	symbols    []string
	token      string
	url        string
	version    int
	threshold  int
	changed    chan struct{}
}

// NewStreamer creates a Tiingo IEX streamer.
func NewStreamer(ctx context.Context, config Config) *Streamer {
	streamCtx, cancel := context.WithCancel(ctx)

	return &Streamer{
		cancel:     cancel,
		ctx:        streamCtx,
		chanError:  config.ChanError,
		chanUpdate: config.ChanUpdate,
		changed:    make(chan struct{}, 1),
		threshold:  config.ThresholdLevel,
		token:      config.Token,
		url:        config.URL,
	}
}

// Start starts the connection manager. It waits for symbols before dialing.
func (s *Streamer) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.isStarted {
		return errors.New("streamer already started")
	}

	if s.url == "" {
		return errors.New("tiingo streaming URL is required")
	}

	if s.token == "" && len(s.symbols) > 0 {
		return errors.New("TIINGO_API_TOKEN is required for .TI symbols")
	}

	s.isStarted = true
	go s.run()

	return nil
}

// SetSymbols replaces the subscription symbols. A reconnect removes old symbols.
func (s *Streamer) SetSymbols(symbols []string, versionVector int) {
	s.mu.Lock()
	s.symbols = slices.Clone(symbols)
	s.version = versionVector
	conn := s.conn
	isStarted := s.isStarted
	s.mu.Unlock()

	if conn != nil {
		_ = conn.Close()
	}

	if isStarted {
		s.notifyChanged()
	}
}

// Stop closes the connection manager and its active websocket.
func (s *Streamer) Stop() {
	s.cancel()

	s.mu.Lock()
	conn := s.conn
	s.mu.Unlock()
	if conn != nil {
		_ = conn.Close()
	}
}

func (s *Streamer) run() {
	for {
		symbols, _ := s.subscription()
		if len(symbols) == 0 {
			select {
			case <-s.ctx.Done():
				return
			case <-s.changed:
				continue
			}
		}

		conn, _, err := websocket.DefaultDialer.DialContext(s.ctx, s.url, nil)
		if err != nil {
			s.reportError(fmt.Errorf("connect Tiingo IEX websocket: %w", err))
			if !s.waitForReconnect() {
				return
			}

			continue
		}

		if !s.setConnection(conn) {
			_ = conn.Close()

			return
		}
		// Symbols may have changed while the websocket handshake was in flight.
		// Subscribe to the latest group rather than the snapshot used to start it.
		symbols, version := s.subscription()
		if len(symbols) == 0 {
			s.clearConnection(conn)
			_ = conn.Close()

			continue
		}

		if err := conn.WriteJSON(s.newSubscription(symbols)); err != nil {
			s.reportError(fmt.Errorf("subscribe to Tiingo IEX websocket: %w", err))
			_ = conn.Close()
			if !s.waitForReconnect() {
				return
			}

			continue
		}

		err = s.readConnection(conn, version)
		s.clearConnection(conn)
		_ = conn.Close()
		if s.ctx.Err() != nil {
			return
		}
		if err != nil {
			s.reportError(fmt.Errorf("read Tiingo IEX websocket: %w", err))
		}
		if !s.waitForReconnect() {
			return
		}
	}
}

func (s *Streamer) readConnection(conn *websocket.Conn, version int) error {
	for {
		var incoming message
		if err := conn.ReadJSON(&incoming); err != nil {
			return err
		}

		updates, err := parseReferencePriceUpdates(incoming)
		if err != nil {
			s.reportError(err)

			continue
		}
		for _, update := range updates {
			select {
			case <-s.ctx.Done():
				return nil
			case s.chanUpdate <- c.MessageUpdate[QuoteUpdate]{
				ID:            update.Symbol,
				Data:          update,
				VersionVector: version,
			}:
			}
		}
	}
}

func (s *Streamer) subscription() ([]string, int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return slices.Clone(s.symbols), s.version
}

func (s *Streamer) newSubscription(symbols []string) subscription {
	return subscription{
		EventName:     "subscribe",
		Authorization: s.token,
		EventData: subscriptionData{
			AuthToken:      s.token,
			Tickers:        symbols,
			ThresholdLevel: s.threshold,
		},
	}
}

func (s *Streamer) setConnection(conn *websocket.Conn) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ctx.Err() != nil {
		return false
	}
	s.conn = conn

	return true
}

func (s *Streamer) clearConnection(conn *websocket.Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conn == conn {
		s.conn = nil
	}
}

func (s *Streamer) waitForReconnect() bool {
	timer := time.NewTimer(reconnectDelay)
	defer timer.Stop()

	select {
	case <-s.ctx.Done():
		return false
	case <-s.changed:
		return true
	case <-timer.C:
		return true
	}
}

func (s *Streamer) notifyChanged() {
	select {
	case s.changed <- struct{}{}:
	default:
	}
}

func (s *Streamer) reportError(err error) {
	select {
	case s.chanError <- err:
	default:
	}
}

func parseReferencePriceUpdates(incoming message) ([]QuoteUpdate, error) {
	if incoming.MessageType != "A" {
		return []QuoteUpdate{}, nil
	}

	return parseReferencePriceData(incoming.Data)
}

func parseReferencePriceData(data json.RawMessage) ([]QuoteUpdate, error) {
	var values []json.RawMessage
	if err := json.Unmarshal(data, &values); err != nil {
		return nil, fmt.Errorf("decode Tiingo IEX websocket data: %w", err)
	}

	if len(values) == 3 {
		var symbol string
		if err := json.Unmarshal(values[1], &symbol); err == nil {
			var price float64
			if err := json.Unmarshal(values[2], &price); err != nil {
				return nil, fmt.Errorf("decode Tiingo IEX reference price: %w", err)
			}

			// Websocket ticks are often lowercase; REST snapshots use uppercase.
			return []QuoteUpdate{{Symbol: strings.ToUpper(symbol), Price: price}}, nil
		}
	}

	updates := make([]QuoteUpdate, 0, len(values))
	for _, value := range values {
		nestedUpdates, err := parseReferencePriceData(value)
		if err != nil {
			return nil, err
		}
		updates = append(updates, nestedUpdates...)
	}

	return updates, nil
}
