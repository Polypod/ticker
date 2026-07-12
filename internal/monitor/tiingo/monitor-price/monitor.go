// Package monitorPriceTiingo combines Tiingo IEX REST snapshots with websocket updates.
package monitorPriceTiingo

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sync"
	"time"

	c "github.com/achannarasappa/ticker/v5/internal/common"
	"github.com/achannarasappa/ticker/v5/internal/monitor/tiingo/monitor-price/streamer"
	"github.com/achannarasappa/ticker/v5/internal/monitor/tiingo/unary"
)

const (
	fromCurrencyCode      = "USD"
	cacheKeyReferenceData = "tiingo:reference-data:"
	ttlReferenceData      = 24 * time.Hour
)

// Config contains the dependencies required by the Tiingo IEX monitor.
type Config struct {
	BaseURL                  string
	Cache                    c.Cache
	ChanError                chan error
	ChanRequestCurrencyRates chan []string
	ChanUpdateAssetQuote     chan c.MessageUpdate[c.AssetQuote]
	Ctx                      context.Context
	StreamingURL             string
	ThresholdLevel           int
	Token                    string
}

// MonitorPriceTiingo manages Tiingo IEX REST snapshots and reference-price updates.
type MonitorPriceTiingo struct {
	assetQuotesCache        []*c.AssetQuote
	assetQuotesCacheLookup  map[string]*c.AssetQuote
	cache                   c.Cache
	chanError               chan error
	chanRequestRates        chan []string
	chanStreamUpdate        chan c.MessageUpdate[streamer.QuoteUpdate]
	chanUpdateAssetQuote    chan c.MessageUpdate[c.AssetQuote]
	currencyRates           c.CurrencyRates
	cancel                  context.CancelFunc
	ctx                     context.Context
	fundamentalsUnavailable bool
	isStarted               bool
	mu                      sync.RWMutex
	referenceData           map[string]unary.ReferenceData
	symbols                 []string
	streamer                *streamer.Streamer
	unaryAPI                *unary.UnaryAPI
	versionVector           int
}

// NewMonitorPriceTiingo creates a Tiingo IEX monitor.
func NewMonitorPriceTiingo(config Config) *MonitorPriceTiingo {
	ctx, cancel := context.WithCancel(config.Ctx)
	updates := make(chan c.MessageUpdate[streamer.QuoteUpdate], 100)

	return &MonitorPriceTiingo{
		assetQuotesCache:       make([]*c.AssetQuote, 0),
		assetQuotesCacheLookup: make(map[string]*c.AssetQuote),
		cache:                  config.Cache,
		cancel:                 cancel,
		chanError:              config.ChanError,
		chanRequestRates:       config.ChanRequestCurrencyRates,
		chanStreamUpdate:       updates,
		chanUpdateAssetQuote:   config.ChanUpdateAssetQuote,
		currencyRates:          make(c.CurrencyRates),
		ctx:                    ctx,
		referenceData:          make(map[string]unary.ReferenceData),
		unaryAPI: unary.NewUnaryAPI(unary.Config{
			BaseURL: config.BaseURL,
			Token:   config.Token,
		}),
		streamer: streamer.NewStreamer(ctx, streamer.Config{
			Token:          config.Token,
			URL:            config.StreamingURL,
			ThresholdLevel: config.ThresholdLevel,
			ChanError:      config.ChanError,
			ChanUpdate:     updates,
		}),
	}
}

// Start retrieves an initial snapshot and begins receiving websocket updates.
func (m *MonitorPriceTiingo) Start() error {
	m.mu.Lock()
	if m.isStarted {
		m.mu.Unlock()

		return errors.New("monitor already started")
	}
	m.isStarted = true
	hasSymbols := len(m.symbols) > 0
	m.mu.Unlock()

	if hasSymbols {
		m.refreshReferenceData(m.currentSymbols())

		if _, err := m.refreshSnapshot(); err != nil {
			m.mu.Lock()
			m.isStarted = false
			m.mu.Unlock()

			return err
		}
	}

	go m.handleStreamUpdates()
	if err := m.streamer.Start(); err != nil {
		m.mu.Lock()
		m.isStarted = false
		m.mu.Unlock()

		return err
	}

	return nil
}

// Stop stops the websocket and prevents future updates.
func (m *MonitorPriceTiingo) Stop() error {
	m.cancel()
	m.streamer.Stop()

	m.mu.Lock()
	m.isStarted = false
	m.mu.Unlock()

	return nil
}

// SetSymbols refreshes the REST snapshot and replaces the websocket subscription.
func (m *MonitorPriceTiingo) SetSymbols(symbols []string, versionVector int) error {
	symbols = slices.Clone(symbols)
	slices.Sort(symbols)
	symbols = slices.Compact(symbols)

	m.mu.Lock()
	m.symbols = symbols
	m.versionVector = versionVector
	m.mu.Unlock()

	if len(symbols) > 0 {
		m.refreshReferenceData(symbols)

		if _, err := m.refreshSnapshot(); err != nil {
			return err
		}
	}

	// Tiingo IEX quotes are USD-denominated. Existing currency conversion can
	// therefore be reused for the rest of ticker's presentation pipeline.
	select {
	case m.chanRequestRates <- []string{fromCurrencyCode}:
	default:
	}

	m.streamer.SetSymbols(symbols, versionVector)

	return nil
}

// GetAssetQuotes returns cached quotes, or obtains a fresh REST snapshot when requested.
func (m *MonitorPriceTiingo) GetAssetQuotes(ignoreCache ...bool) ([]c.AssetQuote, error) {
	if len(ignoreCache) > 0 && ignoreCache[0] {
		quotes, err := m.refreshSnapshot()
		if err != nil {
			return []c.AssetQuote{}, err
		}

		return cloneQuotes(quotes), nil
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	return cloneQuotes(m.assetQuotesCache), nil
}

// SetCurrencyRates updates the rate applied to all cached Tiingo quotes.
func (m *MonitorPriceTiingo) SetCurrencyRates(currencyRates c.CurrencyRates) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.currencyRates = currencyRates
	for _, quote := range m.assetQuotesCache {
		applyCurrencyRate(quote, currencyRates)
	}

	return nil
}

func (m *MonitorPriceTiingo) refreshSnapshot() ([]*c.AssetQuote, error) {
	m.mu.RLock()
	symbols := slices.Clone(m.symbols)
	currencyRates := m.currencyRates
	referenceData := maps.Clone(m.referenceData)
	m.mu.RUnlock()

	assetQuotes, err := m.unaryAPI.GetAssetQuotes(symbols)
	if err != nil {
		return nil, err
	}

	cache := make([]*c.AssetQuote, 0, len(assetQuotes))
	lookup := make(map[string]*c.AssetQuote, len(assetQuotes))
	for _, assetQuote := range assetQuotes {
		quote := assetQuote
		applyReferenceData(&quote, referenceData[quote.Meta.SymbolInSourceAPI])
		applyCurrencyRate(&quote, currencyRates)
		cache = append(cache, &quote)
		lookup[quote.Meta.SymbolInSourceAPI] = &quote
	}

	m.mu.Lock()
	m.assetQuotesCache = cache
	m.assetQuotesCacheLookup = lookup
	m.mu.Unlock()

	return cache, nil
}

func (m *MonitorPriceTiingo) refreshReferenceData(symbols []string) {
	for _, symbol := range symbols {
		if m.hasReferenceData(symbol) {
			continue
		}
		if m.loadReferenceDataFromCache(symbol) {
			continue
		}

		referenceData, err := m.unaryAPI.GetFiftyTwoWeekRange(symbol)
		if err != nil {
			m.reportError(err)

			continue
		}

		if !m.fundamentalsUnavailable {
			marketCapData, marketCapErr := m.unaryAPI.GetMarketCap(symbol)
			if marketCapErr != nil {
				m.fundamentalsUnavailable = true
				m.reportError(marketCapErr)
			} else {
				referenceData.MarketCap = marketCapData.MarketCap
				referenceData.HasMarketCap = marketCapData.HasMarketCap
			}
		}

		m.mu.Lock()
		m.referenceData[symbol] = referenceData
		m.mu.Unlock()
		if m.cache != nil {
			m.cache.Set(referenceDataCacheKey(symbol), referenceData, ttlReferenceData)
		}
	}
}

func (m *MonitorPriceTiingo) hasReferenceData(symbol string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	_, exists := m.referenceData[symbol]

	return exists
}

func (m *MonitorPriceTiingo) loadReferenceDataFromCache(symbol string) bool {
	if m.cache == nil {
		return false
	}

	var referenceData unary.ReferenceData
	if !m.cache.Get(referenceDataCacheKey(symbol), &referenceData) {
		return false
	}

	m.mu.Lock()
	m.referenceData[symbol] = referenceData
	m.mu.Unlock()

	return true
}

func (m *MonitorPriceTiingo) currentSymbols() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return slices.Clone(m.symbols)
}

func (m *MonitorPriceTiingo) handleStreamUpdates() {
	for {
		select {
		case <-m.ctx.Done():
			return
		case update := <-m.chanStreamUpdate:
			m.mu.Lock()
			if update.VersionVector != m.versionVector {
				m.mu.Unlock()

				continue
			}

			assetQuote, exists := m.assetQuotesCacheLookup[update.ID]
			if !exists {
				m.mu.Unlock()

				continue
			}

			assetQuote.QuotePrice.Price = update.Data.Price
			assetQuote.QuotePrice.Change = update.Data.Price - assetQuote.QuotePrice.PricePrevClose
			if assetQuote.QuotePrice.PricePrevClose != 0 {
				assetQuote.QuotePrice.ChangePercent = assetQuote.QuotePrice.Change / assetQuote.QuotePrice.PricePrevClose * 100
			}
			if update.Data.Price > assetQuote.QuotePrice.PriceDayHigh {
				assetQuote.QuotePrice.PriceDayHigh = update.Data.Price
			}
			if assetQuote.QuotePrice.PriceDayLow == 0 || update.Data.Price < assetQuote.QuotePrice.PriceDayLow {
				assetQuote.QuotePrice.PriceDayLow = update.Data.Price
			}
			assetQuote.Exchange.IsActive = true
			quote := *assetQuote
			m.mu.Unlock()

			m.chanUpdateAssetQuote <- c.MessageUpdate[c.AssetQuote]{
				ID:            quote.Symbol,
				Data:          quote,
				VersionVector: update.VersionVector,
			}
		}
	}
}

func applyCurrencyRate(quote *c.AssetQuote, currencyRates c.CurrencyRates) {
	quote.Currency.Rate = 0
	quote.Currency.ToCurrencyCode = ""
	if rate, exists := currencyRates[quote.Currency.FromCurrencyCode]; exists {
		quote.Currency.Rate = rate.Rate
		quote.Currency.ToCurrencyCode = rate.ToCurrency
	}
}

func applyReferenceData(quote *c.AssetQuote, referenceData unary.ReferenceData) {
	if referenceData.HasFiftyTwoWeekRange {
		quote.QuoteExtended.FiftyTwoWeekHigh = referenceData.FiftyTwoWeekHigh
		quote.QuoteExtended.FiftyTwoWeekLow = referenceData.FiftyTwoWeekLow
	}
	if referenceData.HasMarketCap {
		quote.QuoteExtended.MarketCap = referenceData.MarketCap
	}
}

func referenceDataCacheKey(symbol string) string {
	return cacheKeyReferenceData + symbol
}

func (m *MonitorPriceTiingo) reportError(err error) {
	select {
	case m.chanError <- fmt.Errorf("tiingo reference data: %w", err):
	default:
	}
}

func cloneQuotes(quotes []*c.AssetQuote) []c.AssetQuote {
	result := make([]c.AssetQuote, len(quotes))
	for index, quote := range quotes {
		result[index] = *quote
	}

	return result
}
