// Package application define los puertos y casos de uso del hub.
package application

import (
	"context"
	"time"

	"github.com/manuXD270516/go-payment-methods-hub/internal/domain/provider"
	"github.com/manuXD270516/go-payment-methods-hub/internal/domain/token"
)

// Vault guarda el instrumento de pago fuera del alcance del dominio.
type Vault interface {
	Store(ref, pan string) error
	Fingerprint(pan string) string
	Delete(ref string)
}

// TokenRepository persiste los tokens de tarjeta.
type TokenRepository interface {
	Save(ctx context.Context, t token.CardToken) error
	FindByToken(ctx context.Context, merchantID, tok string) (token.CardToken, error)
	Revoke(ctx context.Context, merchantID, tok string, at time.Time) error
}

// ProviderCatalog expone los proveedores configurados.
type ProviderCatalog interface {
	List(ctx context.Context) ([]provider.Provider, error)
}

// OperationRequest es una operación de cobro o pago normalizada.
type OperationRequest struct {
	OperationID string
	MerchantID  string
	Amount      int64
	Currency    string
	Method      provider.MethodType
	Token       string
	Country     string
}

// OperationResult es la respuesta normalizada de un proveedor.
type OperationResult struct {
	ProviderID  string `json:"provider_id"`
	ExternalRef string `json:"external_ref"`
	Status      string `json:"status"`
	Code        string `json:"code"`
}

// ProviderPort es el contrato común que implementa cada proveedor externo.
//
// Agregar un proveedor nuevo consiste en implementar esta interfaz: ni el
// dominio ni los casos de uso cambian.
type ProviderPort interface {
	ID() string
	Charge(ctx context.Context, req OperationRequest) (OperationResult, error)
	Payout(ctx context.Context, req OperationRequest) (OperationResult, error)
}

// IDGenerator genera identificadores opacos.
type IDGenerator interface{ NewID(prefix string) string }

// Clock permite fijar el tiempo en las pruebas.
type Clock interface{ Now() time.Time }
