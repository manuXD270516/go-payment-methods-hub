package providers

import (
	"context"
	"sync"

	"github.com/manuXD270516/go-payment-methods-hub/internal/application"
)

// Simulated es un backend de proveedor sin salida a red, usado en desarrollo
// y pruebas. Un proveedor real implementa la misma interfaz hablando HTTP.
type Simulated struct {
	Identifier string
	// FailTimes hace fallar las primeras N llamadas con un error transitorio,
	// lo que permite ejercitar reintentos y fallback de forma determinista.
	FailTimes int
	// DeclineCode, si no está vacío, rechaza siempre con ese código crudo.
	DeclineCode string

	mu    sync.Mutex
	calls int
}

// Do ejecuta la operación simulada.
func (s *Simulated) Do(_ context.Context, op string, req application.OperationRequest) (application.OperationResult, error) {
	s.mu.Lock()
	s.calls++
	n := s.calls
	s.mu.Unlock()

	if s.DeclineCode != "" {
		return application.OperationResult{}, Normalize(s.Identifier, s.DeclineCode)
	}
	if n <= s.FailTimes {
		return application.OperationResult{}, Normalize(s.Identifier, "SYSTEM_ERROR")
	}
	return application.OperationResult{
		ProviderID:  s.Identifier,
		ExternalRef: s.Identifier + "_" + req.OperationID,
		Status:      "approved",
		Code:        op,
	}, nil
}

// Calls devuelve cuántas veces se invocó al backend.
func (s *Simulated) Calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}
