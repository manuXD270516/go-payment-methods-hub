// Package token modela las tarjetas tokenizadas.
//
// La regla que gobierna este paquete: el número de tarjeta (PAN) entra al
// sistema, se transforma en un token opaco y se descarta. Ninguna estructura de
// este paquete guarda el PAN.
package token

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Errores de dominio de tokenización.
var (
	ErrInvalidPAN    = errors.New("invalid card number")
	ErrInvalidExpiry = errors.New("invalid expiry date")
	ErrNotFound      = errors.New("token not found")
	ErrForbidden     = errors.New("token belongs to another merchant")
)

var soloDigitos = regexp.MustCompile(`^[0-9]{12,19}$`)

// Brand es la marca de la tarjeta, deducida del rango del número.
type Brand string

// Marcas soportadas.
const (
	Visa        Brand = "visa"
	Mastercard  Brand = "mastercard"
	Amex        Brand = "amex"
	Desconocida Brand = "unknown"
)

// CardToken es la representación segura de una tarjeta: identifica el
// instrumento sin permitir reconstruirlo.
type CardToken struct {
	Token       string     `json:"token"`
	MerchantID  string     `json:"merchant_id"`
	Brand       Brand      `json:"brand"`
	Last4       string     `json:"last4"`
	ExpMonth    int        `json:"exp_month"`
	ExpYear     int        `json:"exp_year"`
	Fingerprint string     `json:"fingerprint"`
	CreatedAt   time.Time  `json:"created_at"`
	RevokedAt   *time.Time `json:"revoked_at,omitempty"`
}

// ValidatePAN comprueba formato y dígito verificador de Luhn.
func ValidatePAN(pan string) error {
	limpio := strings.ReplaceAll(strings.TrimSpace(pan), " ", "")
	if !soloDigitos.MatchString(limpio) {
		return ErrInvalidPAN
	}
	if !luhn(limpio) {
		return ErrInvalidPAN
	}
	return nil
}

// luhn implementa el algoritmo de validación de número de tarjeta.
func luhn(pan string) bool {
	suma, doble := 0, false
	for i := len(pan) - 1; i >= 0; i-- {
		d := int(pan[i] - '0')
		if doble {
			if d *= 2; d > 9 {
				d -= 9
			}
		}
		suma += d
		doble = !doble
	}
	return suma%10 == 0
}

// ValidateExpiry rechaza tarjetas vencidas o con fecha imposible.
func ValidateExpiry(month, year int, now time.Time) error {
	if month < 1 || month > 12 {
		return ErrInvalidExpiry
	}
	if year < 100 {
		year += 2000
	}
	vence := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC).AddDate(0, 1, 0)
	if !vence.After(now) {
		return ErrInvalidExpiry
	}
	return nil
}

// DetectBrand deduce la marca a partir del prefijo del número.
func DetectBrand(pan string) Brand {
	switch {
	case strings.HasPrefix(pan, "4"):
		return Visa
	case strings.HasPrefix(pan, "34"), strings.HasPrefix(pan, "37"):
		return Amex
	default:
		if len(pan) >= 2 {
			if n, err := strconv.Atoi(pan[:2]); err == nil && n >= 51 && n <= 55 {
				return Mastercard
			}
		}
		return Desconocida
	}
}

// Last4 devuelve los últimos cuatro dígitos, único fragmento del número que
// puede mostrarse y almacenarse.
func Last4(pan string) string {
	if len(pan) < 4 {
		return ""
	}
	return pan[len(pan)-4:]
}

// IsRevoked indica si el token ya no puede usarse.
func (c CardToken) IsRevoked() bool { return c.RevokedAt != nil }

// BelongsTo verifica el aislamiento multitenant.
func (c CardToken) BelongsTo(merchantID string) bool { return c.MerchantID == merchantID }
