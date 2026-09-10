package http

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/manuXD270516/go-payment-methods-hub/internal/application"
	"github.com/manuXD270516/go-payment-methods-hub/internal/application/usecase"
	"github.com/manuXD270516/go-payment-methods-hub/internal/domain/provider"
	"github.com/manuXD270516/go-payment-methods-hub/internal/infrastructure"
	"github.com/manuXD270516/go-payment-methods-hub/internal/infrastructure/persistence/memory"
	"github.com/manuXD270516/go-payment-methods-hub/internal/infrastructure/providers"
	"github.com/manuXD270516/go-payment-methods-hub/internal/infrastructure/vault"
)

// logCapturado permite comprobar que ningún PAN llega a los registros.
type logCapturado struct{ buf *bytes.Buffer }

func nuevoServidor(t *testing.T) (*httptest.Server, *bytes.Buffer) {
	t.Helper()
	v, err := vault.New("clave-de-prueba-de-32-bytes-o-mas!!")
	if err != nil {
		t.Fatalf("vault: %v", err)
	}
	catalogo := &memory.ProviderCatalog{Providers: []provider.Provider{
		{ID: "prov_bo", Countries: []string{"BO"}, Methods: []provider.MethodType{provider.Card},
			Currencies: []string{"BOB"}, Priority: 1, Health: provider.Healthy},
	}}
	ports := map[string]application.ProviderPort{
		"prov_bo": &providers.Resilient{Identifier: "prov_bo",
			Backend: &providers.Simulated{Identifier: "prov_bo"}, MaxAttempts: 1},
	}
	hub := &usecase.Hub{
		Tokens: memory.NewTokenStore(), Vault: v, Catalog: catalogo, Providers: ports,
		IDs: infrastructure.IDGenerator{}, Clock: infrastructure.SystemClock{},
	}
	buf := new(bytes.Buffer)
	api := &API{Hub: hub, Log: slog.New(slog.NewJSONHandler(buf, nil))}
	srv := httptest.NewServer(api.Routes())
	t.Cleanup(srv.Close)
	return srv, buf
}

func do(t *testing.T, srv *httptest.Server, method, path, body string) (*http.Response, map[string]any) {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, srv.URL+path, r)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	req.Header.Set("X-Merchant-Id", "merch_1")
	req.Header.Set("Content-Type", "application/json")
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	var out map[string]any
	b, _ := io.ReadAll(res.Body)
	res.Body.Close()
	_ = json.Unmarshal(b, &out)
	return res, out
}

const pan = "4242424242424242"

func TestTokenizarViaAPI(t *testing.T) {
	srv, logs := nuevoServidor(t)
	res, body := do(t, srv, http.MethodPost, "/v1/tokens",
		`{"number":"`+pan+`","exp_month":12,"exp_year":2030}`)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d", res.StatusCode)
	}
	if body["last4"] != "4242" || body["brand"] != "visa" {
		t.Fatalf("metadatos = %v", body)
	}
	// La respuesta no debe contener el número completo.
	crudo, _ := json.Marshal(body)
	if strings.Contains(string(crudo), pan) {
		t.Fatal("la respuesta expone el PAN")
	}
	// Los logs tampoco.
	if strings.Contains(logs.String(), pan) {
		t.Fatal("el PAN aparece en los logs")
	}
}

func TestTarjetaInvalidaViaAPI(t *testing.T) {
	srv, _ := nuevoServidor(t)
	res, body := do(t, srv, http.MethodPost, "/v1/tokens",
		`{"number":"4242424242424243","exp_month":12,"exp_year":2030}`)
	if res.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, se esperaba 422", res.StatusCode)
	}
	if body["code"] != "invalid_card_number" {
		t.Errorf("code = %v", body["code"])
	}
}

func TestTarjetaVencida(t *testing.T) {
	srv, _ := nuevoServidor(t)
	pasado := time.Now().AddDate(-1, 0, 0)
	res, body := do(t, srv, http.MethodPost, "/v1/tokens",
		`{"number":"`+pan+`","exp_month":1,"exp_year":`+itoa(pasado.Year())+`}`)
	if res.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, se esperaba 422", res.StatusCode)
	}
	if body["code"] != "invalid_expiry" {
		t.Errorf("code = %v", body["code"])
	}
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

func TestCobroYRastroDeIntentos(t *testing.T) {
	srv, _ := nuevoServidor(t)
	_, tok := do(t, srv, http.MethodPost, "/v1/tokens", `{"number":"`+pan+`","exp_month":12,"exp_year":2030}`)
	res, body := do(t, srv, http.MethodPost, "/v1/charges",
		`{"amount":15000,"currency":"BOB","method":"card","country":"BO","token":"`+tok["token"].(string)+`"}`)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", res.StatusCode)
	}
	decision, _ := body["decision"].(map[string]any)
	if decision["provider_id"] != "prov_bo" || decision["reason"] == "" {
		t.Fatalf("decision incompleta: %v", decision)
	}
	if _, ok := body["attempts"].([]any); !ok {
		t.Fatal("la respuesta debe incluir el rastro de intentos")
	}
}

func TestPreviewDeEnrutamiento(t *testing.T) {
	srv, _ := nuevoServidor(t)
	res, body := do(t, srv, http.MethodGet, "/v1/routing/preview?method=card&country=BO&currency=BOB", "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", res.StatusCode)
	}
	if body["provider_id"] != "prov_bo" {
		t.Fatalf("provider = %v", body["provider_id"])
	}
}

func TestSinProveedorElegible(t *testing.T) {
	srv, _ := nuevoServidor(t)
	res, body := do(t, srv, http.MethodGet, "/v1/routing/preview?method=cash&country=AR&currency=ARS", "")
	if res.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, se esperaba 503", res.StatusCode)
	}
	if body["code"] != "no_eligible_provider" {
		t.Errorf("code = %v", body["code"])
	}
}

func TestRevocarViaAPI(t *testing.T) {
	srv, _ := nuevoServidor(t)
	_, tok := do(t, srv, http.MethodPost, "/v1/tokens", `{"number":"`+pan+`","exp_month":12,"exp_year":2030}`)
	res, _ := do(t, srv, http.MethodDelete, "/v1/tokens/"+tok["token"].(string), "")
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, se esperaba 204", res.StatusCode)
	}
}

func TestSinCabeceraComercio(t *testing.T) {
	srv, _ := nuevoServidor(t)
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/v1/payment-methods", nil)
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, se esperaba 401", res.StatusCode)
	}
}

func TestDocumentacionDisponible(t *testing.T) {
	srv, _ := nuevoServidor(t)
	for _, ruta := range []string{"/healthz", "/readyz", "/openapi.yaml", "/docs"} {
		res, err := srv.Client().Get(srv.URL + ruta)
		if err != nil {
			t.Fatalf("get %s: %v", ruta, err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Errorf("%s devolvio %d", ruta, res.StatusCode)
		}
	}
}
