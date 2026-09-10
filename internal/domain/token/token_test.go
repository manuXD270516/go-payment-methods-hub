package token

import (
	"errors"
	"testing"
	"time"
)

func TestValidatePAN(t *testing.T) {
	tests := []struct {
		name    string
		pan     string
		wantErr bool
	}{
		{"visa valida", "4242424242424242", false},
		{"mastercard valida", "5555555555554444", false},
		{"amex valida", "378282246310005", false},
		{"digito verificador incorrecto", "4242424242424243", true},
		{"con letras", "4242abcd42424242", true},
		{"demasiado corta", "42424", true},
		{"vacia", "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidatePAN(tc.pan)
			if (err != nil) != tc.wantErr {
				t.Fatalf("error = %v, se esperaba error=%v", err, tc.wantErr)
			}
		})
	}
}

func TestDetectBrandYLast4(t *testing.T) {
	casos := map[string]Brand{
		"4242424242424242": Visa,
		"5555555555554444": Mastercard,
		"378282246310005":  Amex,
		"6011111111111117": Desconocida,
	}
	for pan, marca := range casos {
		if got := DetectBrand(pan); got != marca {
			t.Errorf("DetectBrand(%s) = %s, se esperaba %s", pan, got, marca)
		}
	}
	if got := Last4("4242424242424242"); got != "4242" {
		t.Errorf("Last4 = %s", got)
	}
}

func TestValidateExpiry(t *testing.T) {
	ahora := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	if err := ValidateExpiry(12, 2027, ahora); err != nil {
		t.Errorf("fecha futura debe ser valida: %v", err)
	}
	if err := ValidateExpiry(9, 2026, ahora); err != nil {
		t.Errorf("el mes en curso sigue vigente: %v", err)
	}
	if err := ValidateExpiry(8, 2026, ahora); !errors.Is(err, ErrInvalidExpiry) {
		t.Errorf("mes pasado debe fallar, se obtuvo %v", err)
	}
	if err := ValidateExpiry(13, 2027, ahora); !errors.Is(err, ErrInvalidExpiry) {
		t.Errorf("mes invalido debe fallar, se obtuvo %v", err)
	}
}

// El token no debe contener el número completo bajo ninguna circunstancia.
func TestTokenNoContienePAN(t *testing.T) {
	pan := "4242424242424242"
	c := CardToken{Token: "tok_1", Brand: Visa, Last4: Last4(pan), MerchantID: "m1"}
	if c.Last4 == pan {
		t.Fatal("last4 no puede ser el PAN completo")
	}
	if len(c.Last4) != 4 {
		t.Fatalf("last4 debe tener 4 digitos, tiene %d", len(c.Last4))
	}
}
