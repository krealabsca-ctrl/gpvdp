package auth

import (
	"testing"
	"time"
)

func TestJWTRoundtrip(t *testing.T) {
	t.Parallel()
	const secret = "clave-de-prueba"
	tok, err := MintAccessToken(secret, time.Hour, "u1", "a@b.com", "e1", "ADMIN")
	if err != nil {
		t.Fatalf("MintAccessToken: %v", err)
	}
	claims, err := ParseAccessToken(secret, tok)
	if err != nil {
		t.Fatalf("ParseAccessToken: %v", err)
	}
	if claims.UsuarioID() != "u1" {
		t.Errorf("sub = %q, quería u1", claims.UsuarioID())
	}
	if claims.Email != "a@b.com" {
		t.Errorf("email = %q", claims.Email)
	}
	if claims.EmpresaID != "e1" || claims.Rol != "ADMIN" {
		t.Errorf("empresa/rol = %q/%q", claims.EmpresaID, claims.Rol)
	}
	if claims.Tipo != "access" {
		t.Errorf("tipo = %q", claims.Tipo)
	}
}

func TestJWTSecretIncorrecto(t *testing.T) {
	t.Parallel()
	tok, err := MintAccessToken("secreto-a", time.Hour, "u1", "a@b.com", "", "")
	if err != nil {
		t.Fatalf("MintAccessToken: %v", err)
	}
	if _, err := ParseAccessToken("secreto-b", tok); err == nil {
		t.Error("un token firmado con otro secreto debe ser rechazado")
	}
}

func TestJWTExpirado(t *testing.T) {
	t.Parallel()
	const secret = "clave"
	tok, err := MintAccessToken(secret, -time.Minute, "u1", "a@b.com", "", "")
	if err != nil {
		t.Fatalf("MintAccessToken: %v", err)
	}
	if _, err := ParseAccessToken(secret, tok); err == nil {
		t.Error("un token expirado debe ser rechazado")
	}
}
