// Package memory ofrece repositorios en memoria para desarrollo y pruebas.
package memory

import (
	"context"
	"sync"
	"time"

	"github.com/manuXD270516/go-payment-methods-hub/internal/domain/provider"
	"github.com/manuXD270516/go-payment-methods-hub/internal/domain/token"
)

// TokenStore guarda tokens con aislamiento por comercio.
type TokenStore struct {
	mu     sync.RWMutex
	tokens map[string]token.CardToken
}

// NewTokenStore crea un repositorio vacío.
func NewTokenStore() *TokenStore {
	return &TokenStore{tokens: make(map[string]token.CardToken)}
}

// Save persiste un token.
func (s *TokenStore) Save(_ context.Context, t token.CardToken) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokens[t.Token] = t
	return nil
}

// FindByToken recupera un token verificando que pertenezca al comercio.
//
// Un token de otro comercio se trata como inexistente: no se revela que existe.
func (s *TokenStore) FindByToken(_ context.Context, merchantID, tok string) (token.CardToken, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.tokens[tok]
	if !ok || !t.BelongsTo(merchantID) {
		return token.CardToken{}, token.ErrNotFound
	}
	return t, nil
}

// Revoke marca el token como inhabilitado.
func (s *TokenStore) Revoke(_ context.Context, merchantID, tok string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tokens[tok]
	if !ok || !t.BelongsTo(merchantID) {
		return token.ErrNotFound
	}
	t.RevokedAt = &at
	s.tokens[tok] = t
	return nil
}

// ProviderCatalog es un catálogo estático de proveedores.
type ProviderCatalog struct{ Providers []provider.Provider }

// List devuelve los proveedores configurados.
func (c *ProviderCatalog) List(context.Context) ([]provider.Provider, error) {
	return c.Providers, nil
}
