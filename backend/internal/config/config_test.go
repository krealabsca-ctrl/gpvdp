package config

import (
	"strings"
	"testing"

	"github.com/gpvdp/erp/internal/shared"
)

// baseObligatoria pone lo mínimo para que Load no aborte por otra razón.
func baseObligatoria(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://gpvdp:x@db:5432/gpvdp?sslmode=disable")
	t.Setenv("JWT_SECRET", "no-importa-el-valor-en-esta-prueba")
}

// Sin CIFRADO_SECRET el arranque SIGUE. Es la decisión de fondo: hay instalaciones corriendo sin
// la variable y hacerla obligatoria convertiría una mejora del correo en la caída del ERP entero.
func TestLoadSinCifradoSecretNoAborta(t *testing.T) {
	baseObligatoria(t)
	t.Setenv("CIFRADO_SECRET", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("falta CIFRADO_SECRET y Load abortó: %v", err)
	}
	if cfg.CifradoSecret != "" {
		t.Fatalf("se esperaba vacía, vino %q", cfg.CifradoSecret)
	}
	if cfg.CifradoConfigurado() {
		t.Fatal("sin la variable, CifradoConfigurado tiene que ser false")
	}
	// Y el fallo se cobra donde se puede leer: al construir el cifrador, nombrando la variable.
	if _, err := shared.NewCifrador(cfg.CifradoSecret); !strings.Contains(err.Error(), "CIFRADO_SECRET") {
		t.Fatalf("el error tiene que nombrar la variable, dijo %v", err)
	}
}

func TestLoadLeeYRecortaCifradoSecret(t *testing.T) {
	baseObligatoria(t)
	// Con espacios alrededor: un `.env` copiado a mano los trae, y una clave con un espacio de más
	// deriva OTRA clave AES, o sea que todo lo guardado dejaría de descifrar en silencio.
	const clave = "Kf7QmZ2xTb9LrWvA4nJcHs1YpEdGu0Ri6XoNB3kMaVzQwT5yPlCeSgUhIjDfOb8"
	t.Setenv("CIFRADO_SECRET", "  "+clave+"\t")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.CifradoSecret != clave {
		t.Fatalf("no se recortó: %q", cfg.CifradoSecret)
	}
	if !cfg.CifradoConfigurado() {
		t.Fatal("con la variable puesta tiene que ser true")
	}
	// Y el valor tiene que servir de verdad para cifrar, no solo estar presente.
	c, err := shared.NewCifrador(cfg.CifradoSecret)
	if err != nil {
		t.Fatalf("NewCifrador: %v", err)
	}
	if !c.Configurado() {
		t.Fatal("el cifrador tenía que quedar utilizable")
	}
}

// No hay valor de fábrica: una clave escrita en el código no cifraría nada.
func TestCifradoSecretNoTieneDefault(t *testing.T) {
	baseObligatoria(t)
	t.Setenv("CIFRADO_SECRET", "")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.CifradoSecret != "" {
		t.Fatalf("CIFRADO_SECRET no puede tener default, vino %q", cfg.CifradoSecret)
	}
}
