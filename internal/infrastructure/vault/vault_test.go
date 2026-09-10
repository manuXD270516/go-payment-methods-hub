package vault

import (
	"errors"
	"strings"
	"testing"
)

const clave = "clave-de-prueba-de-32-bytes-o-mas!!"

func TestClaveCorta(t *testing.T) {
	if _, err := New("corta"); !errors.Is(err, ErrKeyTooShort) {
		t.Fatalf("se esperaba ErrKeyTooShort, se obtuvo %v", err)
	}
}

// El PAN cifrado no debe aparecer en claro en la representación almacenada.
func TestElPANNoQuedaEnClaro(t *testing.T) {
	v, err := New(clave)
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	const pan = "4242424242424242"
	ref := NewReference()
	if err := v.Store(ref, pan); err != nil {
		t.Fatalf("store: %v", err)
	}
	v.mu.RLock()
	blob := string(v.blobs[ref])
	v.mu.RUnlock()
	if strings.Contains(blob, pan) {
		t.Fatal("el PAN aparece en claro en el almacenamiento")
	}
	if strings.Contains(ref, pan) {
		t.Fatal("la referencia no puede derivarse del PAN")
	}
}

// La huella es estable para la misma tarjeta y distinta para otra, sin
// permitir reconstruir el número.
func TestFingerprintEstableYNoReversible(t *testing.T) {
	v, _ := New(clave)
	a := v.Fingerprint("4242424242424242")
	b := v.Fingerprint("4242424242424242")
	c := v.Fingerprint("5555555555554444")
	if a != b {
		t.Error("la huella debe ser estable para la misma tarjeta")
	}
	if a == c {
		t.Error("tarjetas distintas no pueden compartir huella")
	}
	if strings.Contains(a, "4242") {
		t.Error("la huella no debe contener fragmentos del PAN")
	}
}

func TestDeleteYHas(t *testing.T) {
	v, _ := New(clave)
	ref := NewReference()
	_ = v.Store(ref, "4242424242424242")
	if !v.Has(ref) {
		t.Fatal("el instrumento deberia existir")
	}
	v.Delete(ref)
	if v.Has(ref) {
		t.Fatal("el instrumento deberia haberse eliminado")
	}
}
