// Package providers contiene los adaptadores de proveedores externos.
package providers

import (
	"errors"
	"sync"
	"time"
)

// ErrCircuitOpen indica que el proveedor está temporalmente fuera de servicio.
var ErrCircuitOpen = errors.New("circuit open")

// State es el estado del circuito.
type State string

// Estados del circuit breaker.
const (
	Closed   State = "closed"    // operación normal
	Open     State = "open"      // no se envía tráfico
	HalfOpen State = "half_open" // se permite una prueba
)

// Breaker protege a un proveedor caído de recibir tráfico inútil, y protege al
// sistema de esperar por él.
type Breaker struct {
	mu          sync.Mutex
	state       State
	fallos      int
	umbral      int
	espera      time.Duration
	ultimoFallo time.Time
	now         func() time.Time
}

// NewBreaker crea un circuito cerrado con el umbral y la espera indicados.
func NewBreaker(umbral int, espera time.Duration) *Breaker {
	return &Breaker{state: Closed, umbral: umbral, espera: espera, now: time.Now}
}

// State devuelve el estado actual, aplicando la transición a semiabierto
// cuando ya pasó el periodo de espera.
func (b *Breaker) State() State {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.estadoActual()
}

func (b *Breaker) estadoActual() State {
	if b.state == Open && b.now().Sub(b.ultimoFallo) >= b.espera {
		b.state = HalfOpen
	}
	return b.state
}

// Allow indica si se permite enviar tráfico en este momento.
func (b *Breaker) Allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.estadoActual() != Open
}

// Success registra una operación exitosa y cierra el circuito.
func (b *Breaker) Success() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.fallos, b.state = 0, Closed
}

// Failure registra un fallo y abre el circuito al alcanzar el umbral.
func (b *Breaker) Failure() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.fallos++
	b.ultimoFallo = b.now()
	if b.fallos >= b.umbral || b.state == HalfOpen {
		b.state = Open
	}
}
