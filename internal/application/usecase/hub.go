// Package usecase orquesta las operaciones del hub de métodos de pago.
package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/manuXD270516/go-payment-methods-hub/internal/application"
	"github.com/manuXD270516/go-payment-methods-hub/internal/domain/provider"
	"github.com/manuXD270516/go-payment-methods-hub/internal/domain/token"
)

// ErrAllProvidersFailed indica que ningún proveedor elegible pudo procesar.
var ErrAllProvidersFailed = errors.New("all eligible providers failed")

// Hub agrupa los casos de uso de tokenización, enrutamiento y operaciones.
type Hub struct {
	Tokens    application.TokenRepository
	Vault     application.Vault
	Catalog   application.ProviderCatalog
	Providers map[string]application.ProviderPort
	IDs       application.IDGenerator
	Clock     application.Clock
}

// TokenizeInput son los datos de una tarjeta a tokenizar.
type TokenizeInput struct {
	MerchantID string
	PAN        string
	ExpMonth   int
	ExpYear    int
}

// Tokenize valida la tarjeta, la cifra en el vault y devuelve un token opaco.
//
// El PAN vive únicamente dentro de esta función: no se persiste en el
// repositorio de tokens ni se devuelve al llamador.
func (h *Hub) Tokenize(ctx context.Context, in TokenizeInput) (token.CardToken, error) {
	if err := token.ValidatePAN(in.PAN); err != nil {
		return token.CardToken{}, err
	}
	if err := token.ValidateExpiry(in.ExpMonth, in.ExpYear, h.Clock.Now()); err != nil {
		return token.CardToken{}, err
	}
	tok := h.IDs.NewID("tok")
	if err := h.Vault.Store(tok, in.PAN); err != nil {
		return token.CardToken{}, fmt.Errorf("guardando instrumento: %w", err)
	}
	ct := token.CardToken{
		Token:       tok,
		MerchantID:  in.MerchantID,
		Brand:       token.DetectBrand(in.PAN),
		Last4:       token.Last4(in.PAN),
		ExpMonth:    in.ExpMonth,
		ExpYear:     in.ExpYear,
		Fingerprint: h.Vault.Fingerprint(in.PAN),
		CreatedAt:   h.Clock.Now(),
	}
	if err := h.Tokens.Save(ctx, ct); err != nil {
		h.Vault.Delete(tok)
		return token.CardToken{}, fmt.Errorf("guardando token: %w", err)
	}
	return ct, nil
}

// GetToken devuelve los metadatos de un token del comercio indicado.
func (h *Hub) GetToken(ctx context.Context, merchantID, tok string) (token.CardToken, error) {
	return h.Tokens.FindByToken(ctx, merchantID, tok)
}

// RevokeToken inhabilita un token y borra el instrumento del vault.
func (h *Hub) RevokeToken(ctx context.Context, merchantID, tok string) error {
	if _, err := h.Tokens.FindByToken(ctx, merchantID, tok); err != nil {
		return err
	}
	if err := h.Tokens.Revoke(ctx, merchantID, tok, h.Clock.Now()); err != nil {
		return err
	}
	h.Vault.Delete(tok)
	return nil
}

// PreviewRouting explica qué proveedor se elegiría, sin ejecutar nada.
func (h *Hub) PreviewRouting(ctx context.Context, req provider.Request) (provider.Decision, error) {
	catalogo, err := h.Catalog.List(ctx)
	if err != nil {
		return provider.Decision{}, err
	}
	return provider.Route(catalogo, req)
}

// Attempt documenta cada intento contra un proveedor, exitoso o no.
type Attempt struct {
	ProviderID string `json:"provider_id"`
	Error      string `json:"error,omitempty"`
	Succeeded  bool   `json:"succeeded"`
}

// OperationOutcome es el resultado de una operación con su rastro de intentos.
type OperationOutcome struct {
	Result   application.OperationResult `json:"result"`
	Decision provider.Decision           `json:"decision"`
	Attempts []Attempt                   `json:"attempts"`
}

// Charge cobra usando el proveedor elegido, con fallback a las alternativas.
func (h *Hub) Charge(ctx context.Context, in application.OperationRequest) (OperationOutcome, error) {
	return h.ejecutar(ctx, in, func(p application.ProviderPort, r application.OperationRequest) (application.OperationResult, error) {
		return p.Charge(ctx, r)
	})
}

// Payout paga a un tercero usando el proveedor elegido, con fallback.
func (h *Hub) Payout(ctx context.Context, in application.OperationRequest) (OperationOutcome, error) {
	return h.ejecutar(ctx, in, func(p application.ProviderPort, r application.OperationRequest) (application.OperationResult, error) {
		return p.Payout(ctx, r)
	})
}

// ejecutar aplica la decisión de enrutamiento y, si el proveedor elegido falla,
// recorre las alternativas en orden. Cada intento queda registrado.
func (h *Hub) ejecutar(
	ctx context.Context,
	in application.OperationRequest,
	op func(application.ProviderPort, application.OperationRequest) (application.OperationResult, error),
) (OperationOutcome, error) {
	if in.Token != "" {
		ct, err := h.Tokens.FindByToken(ctx, in.MerchantID, in.Token)
		if err != nil {
			return OperationOutcome{}, err
		}
		if ct.IsRevoked() {
			return OperationOutcome{}, token.ErrNotFound
		}
	}
	decision, err := h.PreviewRouting(ctx, provider.Request{
		Method: in.Method, Country: in.Country, Currency: in.Currency,
	})
	if err != nil {
		return OperationOutcome{}, err
	}
	if in.OperationID == "" {
		in.OperationID = h.IDs.NewID("op")
	}

	orden := append([]string{decision.ProviderID}, decision.Alternatives...)
	out := OperationOutcome{Decision: decision}
	for _, id := range orden {
		p, ok := h.Providers[id]
		if !ok {
			out.Attempts = append(out.Attempts, Attempt{ProviderID: id, Error: "adaptador no registrado"})
			continue
		}
		res, err := op(p, in)
		if err != nil {
			out.Attempts = append(out.Attempts, Attempt{ProviderID: id, Error: err.Error()})
			continue
		}
		out.Attempts = append(out.Attempts, Attempt{ProviderID: id, Succeeded: true})
		out.Result = res
		return out, nil
	}
	return out, ErrAllProvidersFailed
}
