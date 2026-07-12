package tradingbrief

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	ibkr "github.com/osauer/ibkr/pkg/ibkr"
)

var (
	errIBKRGatewayUnavailable = errors.New("IBKR Gateway stream unavailable")
	errIBKRUpdatesPending     = errors.New("IBKR account updates pending")
)

const ibkrReconnectAttemptInterval = 30 * time.Second

// IBKRAccountSnapshot is a normalized, streaming account/position view.
// It deliberately contains no order or market-data request capability.
type IBKRAccountSnapshot struct {
	AccountID          string
	AvailableFunds     float64
	BuyingPower        float64
	Cash               float64
	Currency           string
	GrossPositionValue float64
	Leverage           float64
	NetLiquidation     float64
	Positions          []IBKRPosition
	SettledCash        float64
	Status             string
	Stale              bool
	UnrealizedPnL      float64
	UpdatedAt          time.Time
}

// IBKRPosition is a normalized open position from IBKR's account-update stream.
// Daily P&L is intentionally not requested: it requires a separate P&L stream.
type IBKRPosition struct {
	AverageCost   float64
	DailyPnL      float64
	MarketPrice   float64
	MarketValue   float64
	Quantity      float64
	Symbol        string
	UnrealizedPnL float64
}

// ibkrAccountReader isolates the terminal brief from the broker protocol. Its
// only operation is a cached read backed by the account-update subscription.
type ibkrAccountReader interface {
	Snapshot() (*IBKRAccountSnapshot, error)
}

type ibkrSocketReader struct {
	accountID string
	clientID  int
	host      string
	port      int

	mu          sync.Mutex
	connector   *ibkr.Connector
	lastAttempt time.Time
	lastGood    *IBKRAccountSnapshot
	subscribed  bool
}

func newIBKRSocketReader(host string, port, clientID int, accountID string) *ibkrSocketReader {
	return &ibkrSocketReader{
		accountID: strings.TrimSpace(accountID),
		clientID:  clientID,
		host:      strings.TrimSpace(host),
		port:      port,
	}
}

// Snapshot starts one local TWS/IB Gateway account-update subscription, then
// reads only its in-process cache. It does not call reqMktData, reqPnL,
// reqAccountSummary, or any order endpoint.
func (r *ibkrSocketReader) Snapshot() (*IBKRAccountSnapshot, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.ensureConnected(); err != nil {
		return r.staleSnapshot(err)
	}
	if !r.subscribed {
		if err := r.connector.RequestAccountUpdates(r.accountID); err != nil {
			return r.staleSnapshot(fmt.Errorf("subscribe to account updates: %w", err))
		}
		r.subscribed = true
	}

	summary := r.connector.CachedAccountSummary()
	if summary == nil {
		return r.staleSnapshot(errIBKRUpdatesPending)
	}
	positions, err := r.connector.GetCachedPositions()
	if err != nil {
		return r.staleSnapshot(fmt.Errorf("read cached positions: %w", err))
	}

	snapshot := normalizeIBKRAccount(summary, positions)
	if snapshot.AccountID == "" {
		snapshot.AccountID = r.accountID
	}
	if snapshot.AccountID == "" {
		snapshot.AccountID = r.connector.AccountID()
	}
	snapshot.Status = "Streaming account and portfolio updates"
	r.lastGood = cloneIBKRAccountSnapshot(snapshot)

	return snapshot, nil
}

func (r *ibkrSocketReader) ensureConnected() error {
	if r.connector != nil && r.connector.IsConnected() {
		return nil
	}
	if !r.lastAttempt.IsZero() && time.Since(r.lastAttempt) < ibkrReconnectAttemptInterval {
		return r.gatewayUnavailableError()
	}
	r.lastAttempt = time.Now()
	if r.connector != nil {
		_ = r.connector.Stop()
	}

	config := ibkr.DefaultConfig()
	config.Host = r.host
	config.Port = r.port
	config.ClientID = r.clientID
	config.ConnectTimeout = 3 * time.Second
	r.connector = ibkr.NewConnector(&ibkr.ConnectorConfig{
		ServiceName:       "ticker-read-only-brief",
		PreferredClientID: r.clientID,
		BaseConfig:        config,
	})
	r.subscribed = false

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	if err := r.connector.Start(ctx); err != nil {
		return fmt.Errorf("start IBKR connector: %w", err)
	}
	if !r.connector.IsConnected() {
		return r.gatewayUnavailableError()
	}

	return nil
}

func (r *ibkrSocketReader) gatewayUnavailableError() error {
	detail := ""
	if r.connector != nil {
		detail = r.connector.LastError()
	}
	if detail == "" {
		detail = "connection not established"
	}

	return fmt.Errorf("%w at %s:%d (%s)", errIBKRGatewayUnavailable, r.host, r.port, detail)
}

func (r *ibkrSocketReader) staleSnapshot(err error) (*IBKRAccountSnapshot, error) {
	if r.lastGood == nil {
		return nil, err
	}
	snapshot := cloneIBKRAccountSnapshot(r.lastGood)
	snapshot.Stale = true
	snapshot.Status = "Last IBKR account update retained; gateway stream unavailable"

	return snapshot, err
}

func normalizeIBKRAccount(summary *ibkr.RawAccountSummary, positions []*ibkr.RawPosition) *IBKRAccountSnapshot {
	snapshot := &IBKRAccountSnapshot{
		AccountID:          summary.AccountID,
		AvailableFunds:     floatValue(summary.AvailableFunds),
		BuyingPower:        floatValue(summary.BuyingPower),
		Cash:               floatValue(summary.TotalCashValue),
		Currency:           summary.Currency,
		GrossPositionValue: floatValue(summary.GrossPositionValue),
		NetLiquidation:     floatValue(summary.NetLiquidation),
		SettledCash:        rawFloat(summary.Raw, "SettledCash"),
		UnrealizedPnL:      floatValue(summary.UnrealizedPnL),
		UpdatedAt:          time.Now(),
	}
	if snapshot.NetLiquidation > 0 {
		snapshot.Leverage = snapshot.GrossPositionValue / snapshot.NetLiquidation
	}
	for _, position := range positions {
		if position == nil {
			continue
		}
		snapshot.Positions = append(snapshot.Positions, IBKRPosition{
			AverageCost:   position.AverageCost,
			MarketPrice:   position.MarketPrice,
			MarketValue:   position.MarketValue,
			Quantity:      position.Position,
			Symbol:        position.Contract.Symbol,
			UnrealizedPnL: position.UnrealizedPNL,
		})
	}

	return snapshot
}

func floatValue(value *float64) float64 {
	if value == nil {
		return 0
	}

	return *value
}

func rawFloat(values map[string]string, name string) float64 {
	value, err := strconv.ParseFloat(strings.TrimSpace(values[name]), 64)
	if err != nil {
		return 0
	}

	return value
}

func cloneIBKRAccountSnapshot(source *IBKRAccountSnapshot) *IBKRAccountSnapshot {
	if source == nil {
		return nil
	}
	clone := *source
	clone.Positions = append([]IBKRPosition(nil), source.Positions...)

	return &clone
}
