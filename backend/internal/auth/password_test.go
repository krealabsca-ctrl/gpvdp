package auth

import "testing"

func TestPasswordHashYVerify(t *testing.T) {
	t.Parallel()
	hash, err := HashPassword("secreto-123")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if hash == "secreto-123" {
		t.Fatal("el hash no debe ser igual a la contraseña en claro")
	}
	if !VerifyPassword(hash, "secreto-123") {
		t.Error("VerifyPassword debería aceptar la contraseña correcta")
	}
	if VerifyPassword(hash, "otra-cosa") {
		t.Error("VerifyPassword debería rechazar una contraseña incorrecta")
	}
}
