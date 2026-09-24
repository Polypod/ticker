package server

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	c "github.com/achannarasappa/ticker/v5/internal/common"
	mon "github.com/achannarasappa/ticker/v5/internal/monitor"
)

// Run starts the loopback HTTP API and blocks until interrupted.
func Run(dep *c.Dependencies, ctx *c.Context, options *Options) func(*cobra.Command, []string) {
	return func(_ *cobra.Command, _ []string) {

		if err := serve(dep, ctx, options); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	}
}

func serve(dep *c.Dependencies, ctx *c.Context, options *Options) error {

	token := options.Token

	if token == "" {
		generated, err := GenerateToken()

		if err != nil {
			return err
		}

		token = generated
	}

	monitors, err := mon.NewMonitor(mon.ConfigMonitor{
		RefreshInterval: ctx.Config.RefreshInterval,
		TargetCurrency:  ctx.Config.Currency,
		Logger:          ctx.Logger,
		Cache:           ctx.Cache,
		ConfigMonitorsYahoo: mon.ConfigMonitorsYahoo{
			BaseURL:           dep.MonitorYahooBaseURL,
			SessionRootURL:    dep.MonitorYahooSessionRootURL,
			SessionCrumbURL:   dep.MonitorYahooSessionCrumbURL,
			SessionConsentURL: dep.MonitorYahooSessionConsentURL,
		},
		ConfigMonitorPriceCoinbase: mon.ConfigMonitorPriceCoinbase{
			BaseURL:      dep.MonitorPriceCoinbaseBaseURL,
			StreamingURL: dep.MonitorPriceCoinbaseStreamingURL,
		},
		ConfigMonitorPriceTiingo: mon.ConfigMonitorPriceTiingo{
			BaseURL:        dep.MonitorTiingoBaseURL,
			StreamingURL:   dep.MonitorTiingoStreamingURL,
			Token:          dep.MonitorTiingoToken,
			ThresholdLevel: dep.MonitorTiingoThresholdLevel,
		},
	})

	if err != nil {
		return fmt.Errorf("unable to start monitors: %w", err)
	}

	snapshot := func() Snapshot {
		return NewSnapshot(*ctx, monitors.GetAssetGroupQuote())
	}

	handler, streams := NewHandler(token, snapshot)

	// Every update rebroadcasts the whole snapshot. Clients stay dumb, and the
	// hub's latest-wins slot absorbs bursts from the Tiingo stream.
	// ponytail: full snapshot per update; switch to per-symbol deltas only if a
	// large watchlist makes the payload size show up in practice.
	broadcast := func() {
		streams.broadcast(mustMarshal(snapshot()))
	}

	err = monitors.SetOnUpdate(mon.ConfigUpdateFns{
		OnUpdateAssetQuote: func(_ string, _ c.AssetQuote, _ int) {
			broadcast()
		},
		OnUpdateAssetGroupQuote: func(_ c.AssetGroupQuote, _ int) {
			broadcast()
		},
	})

	if err != nil {
		return err
	}

	if len(ctx.Groups) == 0 {
		return errors.New("no asset groups configured") //nolint:goerr113
	}

	if err = monitors.SetAssetGroup(ctx.Groups[0], 0); err != nil {
		return err
	}

	monitors.Start()
	defer monitors.Stop()

	listener, err := net.Listen("tcp", options.Address)

	if err != nil {
		return fmt.Errorf("unable to listen on %s: %w", options.Address, err)
	}

	discoveryPath, err := WriteDiscovery(listener, token)

	if err != nil {
		return err
	}

	defer os.Remove(discoveryPath) //nolint:errcheck

	fmt.Printf("ticker serve listening on http://%s\n", listener.Addr().String())
	fmt.Printf("token written to %s\n", discoveryPath)

	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	chanSignal := make(chan os.Signal, 1)
	signal.Notify(chanSignal, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-chanSignal
		server.Close() //nolint:errcheck
	}()

	if err = server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("server stopped: %w", err)
	}

	return nil
}
