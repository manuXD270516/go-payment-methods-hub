// Package http expone el hub de métodos de pago como API REST.
package http

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/manuXD270516/go-payment-methods-hub/internal/application"
	"github.com/manuXD270516/go-payment-methods-hub/internal/application/usecase"
	"github.com/manuXD270516/go-payment-methods-hub/internal/domain/provider"
	"github.com/manuXD270516/go-payment-methods-hub/internal/domain/token"
	"github.com/manuXD270516/go-payment-methods-hub/internal/infrastructure/providers"
)

// Problem implementa el formato de error RFC 7807.
type Problem struct {
	Type   string `json:"type"`
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail,omitempty"`
	Code   string `json:"code"`
}

// API agrupa las dependencias de los handlers.
type API struct {
	Hub *usecase.Hub
	Log *slog.Logger
}

const headerMerchant = "X-Merchant-Id"

func merchant(r *http.Request) string { return r.Header.Get(headerMerchant) }

func writeProblem(w http.ResponseWriter, status int, code, title, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Problem{
		Type: "https://docs.paygo.dev/errors/" + code, Title: title,
		Status: status, Detail: detail, Code: code,
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// mapError traduce errores de dominio a códigos HTTP estables.
func mapError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, token.ErrNotFound):
		writeProblem(w, http.StatusNotFound, "token_not_found", "Token no encontrado", "")
	case errors.Is(err, token.ErrInvalidPAN):
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_card_number", "Numero de tarjeta invalido", "")
	case errors.Is(err, token.ErrInvalidExpiry):
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_expiry", "Fecha de vencimiento invalida", "")
	case errors.Is(err, provider.ErrNoProvider):
		writeProblem(w, http.StatusServiceUnavailable, "no_eligible_provider", "Sin proveedor elegible", err.Error())
	case errors.Is(err, providers.ErrInsufficientFunds):
		writeProblem(w, http.StatusPaymentRequired, "insufficient_funds", "Fondos insuficientes", "")
	case errors.Is(err, providers.ErrInstrumentInvalid):
		writeProblem(w, http.StatusPaymentRequired, "instrument_invalid", "Instrumento invalido", "")
	case errors.Is(err, providers.ErrCircuitOpen):
		writeProblem(w, http.StatusServiceUnavailable, "provider_circuit_open", "Proveedor fuera de servicio", "")
	case errors.Is(err, usecase.ErrAllProvidersFailed):
		writeProblem(w, http.StatusBadGateway, "all_providers_failed", "Ningun proveedor pudo procesar", "")
	default:
		writeProblem(w, http.StatusInternalServerError, "internal_error", "Error interno", "")
	}
}

type tokenizeBody struct {
	Number   string `json:"number"`
	ExpMonth int    `json:"exp_month"`
	ExpYear  int    `json:"exp_year"`
}

type operationBody struct {
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
	Method   string `json:"method"`
	Token    string `json:"token"`
	Country  string `json:"country"`
}

func (a *API) tokenize(w http.ResponseWriter, r *http.Request) {
	var in tokenizeBody
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&in); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_json", "Cuerpo invalido", "")
		return
	}
	ct, err := a.Hub.Tokenize(r.Context(), usecase.TokenizeInput{
		MerchantID: merchant(r), PAN: in.Number, ExpMonth: in.ExpMonth, ExpYear: in.ExpYear,
	})
	// El PAN recibido queda fuera de cualquier log: solo se registra el resultado.
	if err != nil {
		mapError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, ct)
}

func (a *API) getToken(w http.ResponseWriter, r *http.Request) {
	ct, err := a.Hub.GetToken(r.Context(), merchant(r), r.PathValue("token"))
	if err != nil {
		mapError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ct)
}

func (a *API) revokeToken(w http.ResponseWriter, r *http.Request) {
	if err := a.Hub.RevokeToken(r.Context(), merchant(r), r.PathValue("token")); err != nil {
		mapError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) routingPreview(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	d, err := a.Hub.PreviewRouting(r.Context(), provider.Request{
		Method:   provider.MethodType(q.Get("method")),
		Country:  q.Get("country"),
		Currency: q.Get("currency"),
	})
	if err != nil {
		mapError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (a *API) operate(charge bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in operationBody
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&in); err != nil {
			writeProblem(w, http.StatusBadRequest, "invalid_json", "Cuerpo invalido", "")
			return
		}
		req := application.OperationRequest{
			MerchantID: merchant(r), Amount: in.Amount, Currency: in.Currency,
			Method: provider.MethodType(in.Method), Token: in.Token, Country: in.Country,
		}
		var (
			out usecase.OperationOutcome
			err error
		)
		if charge {
			out, err = a.Hub.Charge(r.Context(), req)
		} else {
			out, err = a.Hub.Payout(r.Context(), req)
		}
		if err != nil {
			mapError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func (a *API) listMethods(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"methods": []string{string(provider.Card), string(provider.BankTransfer), string(provider.Cash), string(provider.QR)},
	})
}

func requireMerchant(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if merchant(r) == "" {
			writeProblem(w, http.StatusUnauthorized, "merchant_required", "Falta la cabecera X-Merchant-Id", "")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Routes construye el enrutador con API, documentación y health checks.
func (a *API) Routes() http.Handler {
	negocio := http.NewServeMux()
	negocio.HandleFunc("GET /v1/payment-methods", a.listMethods)
	negocio.HandleFunc("POST /v1/tokens", a.tokenize)
	negocio.HandleFunc("GET /v1/tokens/{token}", a.getToken)
	negocio.HandleFunc("DELETE /v1/tokens/{token}", a.revokeToken)
	negocio.HandleFunc("POST /v1/charges", a.operate(true))
	negocio.HandleFunc("POST /v1/payouts", a.operate(false))
	negocio.HandleFunc("GET /v1/routing/preview", a.routingPreview)

	mux := http.NewServeMux()
	mux.Handle("/v1/", requireMerchant(negocio))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})
	mux.HandleFunc("GET /openapi.yaml", serveOpenAPI)
	mux.HandleFunc("GET /docs", serveSwaggerUI)

	return logRequests(a.Log, mux)
}

func logRequests(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		inicio := time.Now()
		next.ServeHTTP(w, r)
		// Se registran metadatos, nunca el cuerpo: podría contener un PAN.
		log.Info("request", "method", r.Method, "path", r.URL.Path,
			"merchant", merchant(r), "duration_ms", time.Since(inicio).Milliseconds())
	})
}
