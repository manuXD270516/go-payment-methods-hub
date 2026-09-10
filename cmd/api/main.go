// Comando api arranca el hub de métodos de pago.
package main

import (
	"context"
	"errors"
	"log/slog"
	stdhttp "net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/manuXD270516/go-payment-methods-hub/internal/application"
	"github.com/manuXD270516/go-payment-methods-hub/internal/application/usecase"
	"github.com/manuXD270516/go-payment-methods-hub/internal/domain/provider"
	"github.com/manuXD270516/go-payment-methods-hub/internal/infrastructure"
	apihttp "github.com/manuXD270516/go-payment-methods-hub/internal/infrastructure/http"
	"github.com/manuXD270516/go-payment-methods-hub/internal/infrastructure/persistence/memory"
	"github.com/manuXD270516/go-payment-methods-hub/internal/infrastructure/providers"
	"github.com/manuXD270516/go-payment-methods-hub/internal/infrastructure/vault"
	"github.com/manuXD270516/go-payment-methods-hub/internal/platform/config"
)

func main() {
	cfg := config.Load()
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: nivel(cfg.LogLevel)}))

	v, err := vault.New(cfg.VaultKey)
	if err != nil {
		log.Error("no se pudo inicializar el vault", "error", err)
		os.Exit(1)
	}

	// Catálogo de proveedores. En producción vendría de la base de datos.
	catalogo := &memory.ProviderCatalog{Providers: []provider.Provider{
		{ID: "prov_bo", Name: "BoliviaPay", Countries: []string{"BO"},
			Methods:    []provider.MethodType{provider.Card, provider.QR, provider.BankTransfer},
			Currencies: []string{"BOB"}, Priority: 1, Health: provider.Healthy},
		{ID: "prov_andes", Name: "AndesPay", Countries: []string{"BO", "PE", "CO"},
			Methods:    []provider.MethodType{provider.Card, provider.BankTransfer},
			Currencies: []string{"BOB", "PEN", "COP"}, Priority: 2, Health: provider.Healthy},
	}}

	// Cada proveedor se envuelve con su propia política de resiliencia: el
	// circuito de uno no afecta al otro.
	ports := map[string]application.ProviderPort{}
	for _, p := range catalogo.Providers {
		ports[p.ID] = &providers.Resilient{
			Identifier:  p.ID,
			Backend:     &providers.Simulated{Identifier: p.ID},
			Breaker:     providers.NewBreaker(cfg.BreakerThreshold, 30*time.Second),
			MaxAttempts: 3,
			BaseDelay:   100 * time.Millisecond,
		}
	}

	hub := &usecase.Hub{
		Tokens: memory.NewTokenStore(), Vault: v, Catalog: catalogo, Providers: ports,
		IDs: infrastructure.IDGenerator{}, Clock: infrastructure.SystemClock{},
	}
	api := &apihttp.API{Hub: hub, Log: log}

	srv := &stdhttp.Server{
		Addr:              ":" + cfg.HTTPPort,
		Handler:           api.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      20 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	go func() {
		log.Info("hub escuchando", "port", cfg.HTTPPort, "docs", "/docs")
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, stdhttp.ErrServerClosed) {
			log.Error("fallo del servidor", "error", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Info("apagando hub")
	ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Error("apagado forzado", "error", err)
	}
}

func nivel(s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
