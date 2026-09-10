package providers

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/manuXD270516/go-payment-methods-hub/internal/application"
)

// Errores normalizados: los códigos de los terceros se traducen a este
// vocabulario para que el consumidor programe contra un contrato estable.
var (
	ErrInsufficientFunds = errors.New("insufficient_funds")
	ErrInstrumentInvalid = errors.New("instrument_invalid")
	ErrProviderUnavail   = errors.New("provider_unavailable")
)

// Normalize traduce el código crudo de un proveedor a un error de dominio.
func Normalize(providerID, raw string) error {
	switch raw {
	case "51", "NSF", "insufficient_funds", "FONDOS_INSUF":
		return fmt.Errorf("%w (%s)", ErrInsufficientFunds, providerID)
	case "14", "INVALID_CARD", "instrument_invalid":
		return fmt.Errorf("%w (%s)", ErrInstrumentInvalid, providerID)
	case "96", "SYSTEM_ERROR", "unavailable":
		return fmt.Errorf("%w (%s)", ErrProviderUnavail, providerID)
	default:
		return fmt.Errorf("error no clasificado %q (%s)", raw, providerID)
	}
}

// Backend es la operación cruda de un proveedor, antes de la resiliencia.
type Backend interface {
	Do(ctx context.Context, op string, req application.OperationRequest) (application.OperationResult, error)
}

// Resilient envuelve a un proveedor con reintentos y circuit breaker.
//
// Solo se reintentan los errores transitorios: reintentar un rechazo definitivo
// (fondos insuficientes) es inútil y puede resultar sospechoso.
type Resilient struct {
	Identifier  string
	Backend     Backend
	Breaker     *Breaker
	MaxAttempts int
	BaseDelay   time.Duration
}

// ID devuelve el identificador del proveedor.
func (r *Resilient) ID() string { return r.Identifier }

// Charge ejecuta un cobro con la política de resiliencia.
func (r *Resilient) Charge(ctx context.Context, req application.OperationRequest) (application.OperationResult, error) {
	return r.ejecutar(ctx, "charge", req)
}

// Payout ejecuta un pago con la política de resiliencia.
func (r *Resilient) Payout(ctx context.Context, req application.OperationRequest) (application.OperationResult, error) {
	return r.ejecutar(ctx, "payout", req)
}

func (r *Resilient) ejecutar(ctx context.Context, op string, req application.OperationRequest) (application.OperationResult, error) {
	if r.Breaker != nil && !r.Breaker.Allow() {
		return application.OperationResult{}, fmt.Errorf("%w: %s", ErrCircuitOpen, r.Identifier)
	}
	intentos := r.MaxAttempts
	if intentos < 1 {
		intentos = 1
	}
	var ultimo error
	for i := 1; i <= intentos; i++ {
		res, err := r.Backend.Do(ctx, op, req)
		if err == nil {
			if r.Breaker != nil {
				r.Breaker.Success()
			}
			return res, nil
		}
		ultimo = err
		if !esTransitorio(err) {
			break // un rechazo definitivo no se reintenta
		}
		if i < intentos {
			select {
			case <-time.After(r.backoff(i)):
			case <-ctx.Done():
				return application.OperationResult{}, ctx.Err()
			}
		}
	}
	if r.Breaker != nil && esTransitorio(ultimo) {
		r.Breaker.Failure()
	}
	return application.OperationResult{}, ultimo
}

// backoff crece de forma exponencial y añade jitter para que muchos reintentos
// no coincidan en el mismo instante.
func (r *Resilient) backoff(intento int) time.Duration {
	base := r.BaseDelay
	if base == 0 {
		base = 50 * time.Millisecond
	}
	d := time.Duration(math.Pow(2, float64(intento-1))) * base
	return d + time.Duration(rand.Int63n(int64(d/2+1)))
}

func esTransitorio(err error) bool {
	return errors.Is(err, ErrProviderUnavail) || errors.Is(err, context.DeadlineExceeded)
}
