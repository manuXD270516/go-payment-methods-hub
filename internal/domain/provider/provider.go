// Package provider modela los proveedores de pago y la política de enrutamiento.
package provider

import (
	"errors"
	"fmt"
	"sort"
)

// Errores de dominio del enrutamiento.
var (
	ErrNoProvider  = errors.New("no eligible provider")
	ErrUnsupported = errors.New("unsupported method or country")
)

// MethodType identifica la forma de cobro.
type MethodType string

// Métodos soportados por el hub.
const (
	Card         MethodType = "card"
	BankTransfer MethodType = "bank_transfer"
	Cash         MethodType = "cash"
	QR           MethodType = "qr"
)

// Health es el estado operativo de un proveedor.
type Health string

// Estados posibles del circuito de un proveedor.
const (
	Healthy  Health = "healthy"
	Degraded Health = "degraded"
	Down     Health = "down"
)

// Provider describe un proveedor y su alcance.
type Provider struct {
	ID         string       `json:"id"`
	Name       string       `json:"name"`
	Countries  []string     `json:"countries"`
	Methods    []MethodType `json:"methods"`
	Currencies []string     `json:"currencies"`
	Priority   int          `json:"priority"` // menor es mejor
	Health     Health       `json:"health"`
}

// Supports indica si el proveedor cubre el método, país y moneda pedidos.
func (p Provider) Supports(m MethodType, country, currency string) bool {
	return contiene(p.Countries, country) &&
		contieneMetodo(p.Methods, m) &&
		contiene(p.Currencies, currency)
}

// Eligible añade a Supports la condición de estar operativo.
func (p Provider) Eligible(m MethodType, country, currency string) bool {
	return p.Health != Down && p.Supports(m, country, currency)
}

// Request son los datos de entrada de una decisión de enrutamiento.
type Request struct {
	Method   MethodType `json:"method"`
	Country  string     `json:"country"`
	Currency string     `json:"currency"`
}

// Decision es el resultado del enrutamiento. Incluye el motivo y las
// alternativas: la decisión debe poder explicarse a soporte y auditoría, no
// ser una caja negra.
type Decision struct {
	Request      Request  `json:"request"`
	ProviderID   string   `json:"provider_id"`
	Reason       string   `json:"reason"`
	Alternatives []string `json:"alternatives"`
}

// Route elige el proveedor de menor prioridad entre los elegibles.
//
// El resultado es determinista: ante la misma entrada y el mismo catálogo,
// siempre se elige el mismo proveedor. El desempate por ID evita que el orden
// de un map altere la decisión.
func Route(providers []Provider, req Request) (Decision, error) {
	var elegibles []Provider
	for _, p := range providers {
		if p.Eligible(req.Method, req.Country, req.Currency) {
			elegibles = append(elegibles, p)
		}
	}
	if len(elegibles) == 0 {
		return Decision{}, fmt.Errorf("%w: %s/%s/%s", ErrNoProvider, req.Method, req.Country, req.Currency)
	}
	sort.Slice(elegibles, func(i, j int) bool {
		if elegibles[i].Priority != elegibles[j].Priority {
			return elegibles[i].Priority < elegibles[j].Priority
		}
		return elegibles[i].ID < elegibles[j].ID
	})
	elegido := elegibles[0]
	alternativas := make([]string, 0, len(elegibles)-1)
	for _, p := range elegibles[1:] {
		alternativas = append(alternativas, p.ID)
	}
	return Decision{
		Request:    req,
		ProviderID: elegido.ID,
		Reason: fmt.Sprintf("prioridad %d, estado %s, cubre %s en %s/%s",
			elegido.Priority, elegido.Health, req.Method, req.Country, req.Currency),
		Alternatives: alternativas,
	}, nil
}

func contiene(xs []string, v string) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}

func contieneMetodo(xs []MethodType, v MethodType) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}
