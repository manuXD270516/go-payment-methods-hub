package usecase

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/manuXD270516/go-payment-methods-hub/internal/application"
	"github.com/manuXD270516/go-payment-methods-hub/internal/domain/provider"
	"github.com/manuXD270516/go-payment-methods-hub/internal/domain/token"
	"github.com/manuXD270516/go-payment-methods-hub/internal/infrastructure/persistence/memory"
	"github.com/manuXD270516/go-payment-methods-hub/internal/infrastructure/providers"
	"github.com/manuXD270516/go-payment-methods-hub/internal/infrastructure/vault"
)

type reloj struct{ t time.Time }

func (r reloj) Now() time.Time { return r.t }

type ids struct {
	mu sync.Mutex
	n  int
}

func (g *ids) NewID(p string) string {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.n++
	return p + "_" + string(rune('a'+g.n%26)) + string(rune('0'+g.n%10))
}

func catalogo() []provider.Provider {
	return []provider.Provider{
		{ID: "prov_a", Countries: []string{"BO"}, Methods: []provider.MethodType{provider.Card},
			Currencies: []string{"BOB"}, Priority: 1, Health: provider.Healthy},
		{ID: "prov_b", Countries: []string{"BO"}, Methods: []provider.MethodType{provider.Card},
			Currencies: []string{"BOB"}, Priority: 2, Health: provider.Healthy},
	}
}

func nuevoHub(t *testing.T, ports map[string]application.ProviderPort) *Hub {
	t.Helper()
	v, err := vault.New("clave-de-prueba-de-32-bytes-o-mas!!")
	if err != nil {
		t.Fatalf("vault: %v", err)
	}
	return &Hub{
		Tokens:    memory.NewTokenStore(),
		Vault:     v,
		Catalog:   &memory.ProviderCatalog{Providers: catalogo()},
		Providers: ports,
		IDs:       &ids{},
		Clock:     reloj{t: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)},
	}
}

func tokenizar(t *testing.T, h *Hub, merchant string) token.CardToken {
	t.Helper()
	ct, err := h.Tokenize(context.Background(), TokenizeInput{
		MerchantID: merchant, PAN: "4242424242424242", ExpMonth: 12, ExpYear: 2028,
	})
	if err != nil {
		t.Fatalf("tokenizar: %v", err)
	}
	return ct
}

func TestTokenizarNoExponeElPAN(t *testing.T) {
	h := nuevoHub(t, nil)
	ct := tokenizar(t, h, "merch_1")

	if ct.Last4 != "4242" || ct.Brand != token.Visa {
		t.Fatalf("metadatos incorrectos: %s %s", ct.Last4, ct.Brand)
	}
	// Ninguna representación del token puede contener el número completo.
	if strings.Contains(ct.Token, "4242424242424242") ||
		strings.Contains(ct.Fingerprint, "4242424242424242") {
		t.Fatal("el token expone el PAN")
	}
}

func TestTarjetaInvalidaEsRechazada(t *testing.T) {
	h := nuevoHub(t, nil)
	_, err := h.Tokenize(context.Background(), TokenizeInput{
		MerchantID: "m1", PAN: "4242424242424243", ExpMonth: 12, ExpYear: 2028,
	})
	if !errors.Is(err, token.ErrInvalidPAN) {
		t.Fatalf("se esperaba ErrInvalidPAN, se obtuvo %v", err)
	}
}

func TestTokenAisladoPorComercio(t *testing.T) {
	h := nuevoHub(t, nil)
	ct := tokenizar(t, h, "merch_1")
	if _, err := h.GetToken(context.Background(), "merch_2", ct.Token); !errors.Is(err, token.ErrNotFound) {
		t.Fatalf("otro comercio no debe acceder al token, se obtuvo %v", err)
	}
}

func TestRevocarToken(t *testing.T) {
	h := nuevoHub(t, nil)
	ct := tokenizar(t, h, "merch_1")
	if err := h.RevokeToken(context.Background(), "merch_1", ct.Token); err != nil {
		t.Fatalf("revocar: %v", err)
	}
	revocado, err := h.GetToken(context.Background(), "merch_1", ct.Token)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !revocado.IsRevoked() {
		t.Fatal("el token deberia estar revocado")
	}
}

func TestFallbackAlSiguienteProveedor(t *testing.T) {
	caido := &providers.Simulated{Identifier: "prov_a", FailTimes: 99}
	sano := &providers.Simulated{Identifier: "prov_b"}
	h := nuevoHub(t, map[string]application.ProviderPort{
		"prov_a": &providers.Resilient{Identifier: "prov_a", Backend: caido, MaxAttempts: 1, BaseDelay: time.Millisecond},
		"prov_b": &providers.Resilient{Identifier: "prov_b", Backend: sano, MaxAttempts: 1, BaseDelay: time.Millisecond},
	})
	ct := tokenizar(t, h, "merch_1")

	out, err := h.Charge(context.Background(), application.OperationRequest{
		MerchantID: "merch_1", Amount: 10000, Currency: "BOB",
		Method: provider.Card, Token: ct.Token, Country: "BO",
	})
	if err != nil {
		t.Fatalf("charge: %v", err)
	}
	if out.Result.ProviderID != "prov_b" {
		t.Fatalf("debio caer al secundario, uso %s", out.Result.ProviderID)
	}
	// Ambos intentos deben quedar registrados, incluido el fallido.
	if len(out.Attempts) != 2 || out.Attempts[0].Succeeded || !out.Attempts[1].Succeeded {
		t.Fatalf("rastro de intentos incorrecto: %+v", out.Attempts)
	}
}

func TestReintentaSoloErroresTransitorios(t *testing.T) {
	t.Run("transitorio se reintenta", func(t *testing.T) {
		backend := &providers.Simulated{Identifier: "prov_a", FailTimes: 2}
		p := &providers.Resilient{Identifier: "prov_a", Backend: backend, MaxAttempts: 3, BaseDelay: time.Millisecond}
		h := nuevoHub(t, map[string]application.ProviderPort{"prov_a": p})
		if _, err := h.Charge(context.Background(), application.OperationRequest{
			MerchantID: "m", Amount: 100, Currency: "BOB", Method: provider.Card, Country: "BO",
		}); err != nil {
			t.Fatalf("charge: %v", err)
		}
		if backend.Calls() != 3 {
			t.Fatalf("se esperaban 3 llamadas (2 fallos + 1 exito), hubo %d", backend.Calls())
		}
	})

	t.Run("rechazo definitivo no se reintenta", func(t *testing.T) {
		backend := &providers.Simulated{Identifier: "prov_a", DeclineCode: "51"}
		p := &providers.Resilient{Identifier: "prov_a", Backend: backend, MaxAttempts: 3, BaseDelay: time.Millisecond}
		h := nuevoHub(t, map[string]application.ProviderPort{"prov_a": p})
		_, _ = h.Charge(context.Background(), application.OperationRequest{
			MerchantID: "m", Amount: 100, Currency: "BOB", Method: provider.Card, Country: "BO",
		})
		if backend.Calls() != 1 {
			t.Fatalf("un rechazo definitivo no debe reintentarse, hubo %d llamadas", backend.Calls())
		}
	})
}

func TestErroresNormalizadosEntreProveedores(t *testing.T) {
	// Dos proveedores distintos, con códigos crudos distintos para el mismo
	// problema, deben producir el mismo error de dominio.
	a := providers.Normalize("prov_a", "51")
	b := providers.Normalize("prov_b", "FONDOS_INSUF")
	if !errors.Is(a, providers.ErrInsufficientFunds) || !errors.Is(b, providers.ErrInsufficientFunds) {
		t.Fatalf("ambos deben normalizar a fondos insuficientes: %v / %v", a, b)
	}
}

func TestTodosLosProveedoresFallan(t *testing.T) {
	h := nuevoHub(t, map[string]application.ProviderPort{
		"prov_a": &providers.Resilient{Identifier: "prov_a", Backend: &providers.Simulated{Identifier: "prov_a", DeclineCode: "51"}, MaxAttempts: 1},
		"prov_b": &providers.Resilient{Identifier: "prov_b", Backend: &providers.Simulated{Identifier: "prov_b", DeclineCode: "51"}, MaxAttempts: 1},
	})
	_, err := h.Charge(context.Background(), application.OperationRequest{
		MerchantID: "m", Amount: 100, Currency: "BOB", Method: provider.Card, Country: "BO",
	})
	if !errors.Is(err, ErrAllProvidersFailed) {
		t.Fatalf("se esperaba ErrAllProvidersFailed, se obtuvo %v", err)
	}
}

func TestPreviewExplicaLaDecision(t *testing.T) {
	h := nuevoHub(t, nil)
	d, err := h.PreviewRouting(context.Background(), provider.Request{
		Method: provider.Card, Country: "BO", Currency: "BOB",
	})
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if d.ProviderID != "prov_a" || d.Reason == "" {
		t.Fatalf("decision incompleta: %+v", d)
	}
}

// Test de arquitectura: un proveedor nuevo se agrega implementando el puerto,
// sin modificar el dominio ni los casos de uso.
type proveedorNuevo struct{ llamado bool }

func (p *proveedorNuevo) ID() string { return "prov_nuevo" }
func (p *proveedorNuevo) Charge(context.Context, application.OperationRequest) (application.OperationResult, error) {
	p.llamado = true
	return application.OperationResult{ProviderID: "prov_nuevo", Status: "approved"}, nil
}
func (p *proveedorNuevo) Payout(context.Context, application.OperationRequest) (application.OperationResult, error) {
	return application.OperationResult{ProviderID: "prov_nuevo", Status: "approved"}, nil
}

func TestAgregarProveedorNoTocaElNucleo(t *testing.T) {
	nuevo := &proveedorNuevo{}
	h := nuevoHub(t, map[string]application.ProviderPort{"prov_a": nuevo})
	// Se registra bajo el id que el catálogo enruta; basta implementar el puerto.
	if _, err := h.Charge(context.Background(), application.OperationRequest{
		MerchantID: "m", Amount: 100, Currency: "BOB", Method: provider.Card, Country: "BO",
	}); err != nil {
		t.Fatalf("charge: %v", err)
	}
	if !nuevo.llamado {
		t.Fatal("el proveedor nuevo debio ser invocado")
	}
}
