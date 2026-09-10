package providers

import (
	"testing"
	"time"
)

func TestAbreTrasElUmbral(t *testing.T) {
	b := NewBreaker(3, time.Minute)
	for i := 0; i < 2; i++ {
		b.Failure()
	}
	if !b.Allow() {
		t.Fatal("con fallos bajo el umbral el circuito sigue cerrado")
	}
	b.Failure()
	if b.Allow() {
		t.Fatal("alcanzado el umbral, el circuito debe abrir")
	}
	if b.State() != Open {
		t.Fatalf("estado = %s", b.State())
	}
}

func TestPasaASemiabiertoYSeCierra(t *testing.T) {
	b := NewBreaker(1, 50*time.Millisecond)
	ahora := time.Now()
	b.now = func() time.Time { return ahora }

	b.Failure()
	if b.Allow() {
		t.Fatal("debe estar abierto")
	}
	ahora = ahora.Add(60 * time.Millisecond)
	if !b.Allow() {
		t.Fatal("pasada la espera debe permitir una prueba")
	}
	if b.State() != HalfOpen {
		t.Fatalf("estado = %s, se esperaba half_open", b.State())
	}
	b.Success()
	if b.State() != Closed {
		t.Fatalf("tras exito debe cerrarse, estado = %s", b.State())
	}
}

func TestFalloEnSemiabiertoReabre(t *testing.T) {
	b := NewBreaker(1, 10*time.Millisecond)
	ahora := time.Now()
	b.now = func() time.Time { return ahora }
	b.Failure()
	ahora = ahora.Add(20 * time.Millisecond)
	_ = b.Allow() // pasa a semiabierto
	b.Failure()
	if b.State() != Open {
		t.Fatalf("un fallo en semiabierto debe reabrir, estado = %s", b.State())
	}
}
