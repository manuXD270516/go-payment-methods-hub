package provider

import (
	"errors"
	"testing"
)

func catalogo() []Provider {
	return []Provider{
		{ID: "prov_bo", Name: "BoliviaPay", Countries: []string{"BO"}, Methods: []MethodType{Card, QR},
			Currencies: []string{"BOB"}, Priority: 1, Health: Healthy},
		{ID: "prov_regional", Name: "AndesPay", Countries: []string{"BO", "PE", "CO"}, Methods: []MethodType{Card, BankTransfer},
			Currencies: []string{"BOB", "PEN", "COP"}, Priority: 2, Health: Healthy},
		{ID: "prov_caido", Name: "DownPay", Countries: []string{"BO"}, Methods: []MethodType{Card},
			Currencies: []string{"BOB"}, Priority: 0, Health: Down},
	}
}

func TestEligeElDeMayorPrioridadOperativo(t *testing.T) {
	d, err := Route(catalogo(), Request{Method: Card, Country: "BO", Currency: "BOB"})
	if err != nil {
		t.Fatalf("route: %v", err)
	}
	// prov_caido tiene prioridad 0 pero está caído: no debe elegirse.
	if d.ProviderID != "prov_bo" {
		t.Fatalf("proveedor = %s, se esperaba prov_bo", d.ProviderID)
	}
	if d.Reason == "" {
		t.Error("la decision debe explicar su motivo")
	}
	if len(d.Alternatives) != 1 || d.Alternatives[0] != "prov_regional" {
		t.Errorf("alternativas = %v", d.Alternatives)
	}
}

func TestEsDeterminista(t *testing.T) {
	req := Request{Method: Card, Country: "BO", Currency: "BOB"}
	primera, err := Route(catalogo(), req)
	if err != nil {
		t.Fatalf("route: %v", err)
	}
	for i := 0; i < 50; i++ {
		d, err := Route(catalogo(), req)
		if err != nil {
			t.Fatalf("route: %v", err)
		}
		if d.ProviderID != primera.ProviderID {
			t.Fatalf("decision no determinista: %s vs %s", d.ProviderID, primera.ProviderID)
		}
	}
}

func TestSinProveedorElegible(t *testing.T) {
	_, err := Route(catalogo(), Request{Method: Cash, Country: "AR", Currency: "ARS"})
	if !errors.Is(err, ErrNoProvider) {
		t.Fatalf("se esperaba ErrNoProvider, se obtuvo %v", err)
	}
}

func TestEnrutaPorPais(t *testing.T) {
	d, err := Route(catalogo(), Request{Method: BankTransfer, Country: "PE", Currency: "PEN"})
	if err != nil {
		t.Fatalf("route: %v", err)
	}
	if d.ProviderID != "prov_regional" {
		t.Fatalf("proveedor = %s, se esperaba prov_regional", d.ProviderID)
	}
}
