package bancos

import (
	"os"
	"path/filepath"
	"testing"
)

// TestMuestrasReales corre los adaptadores contra los Excel reales si GPVDP_MUESTRAS
// apunta a la carpeta de muestras. Se omite (skip) donde esa carpeta no exista (CI, otros equipos).
func TestMuestrasReales(t *testing.T) {
	dir := os.Getenv("GPVDP_MUESTRAS")
	if dir == "" {
		t.Skip("GPVDP_MUESTRAS no seteado; se omite la prueba con archivos reales")
	}
	files, err := filepath.Glob(filepath.Join(dir, "*.xlsx"))
	if err != nil || len(files) == 0 {
		t.Skipf("sin .xlsx en %s", dir)
	}
	for _, path := range files {
		t.Run(filepath.Base(path), func(t *testing.T) {
			f, err := os.Open(path)
			if err != nil {
				t.Fatalf("abrir: %v", err)
			}
			defer func() { _ = f.Close() }()

			g, err := CargarGrid(f)
			if err != nil {
				t.Fatalf("CargarGrid: %v", err)
			}
			a, err := Detectar(g)
			if err != nil {
				t.Fatalf("Detectar: %v", err)
			}
			res, err := a.Parsea(g)
			if err != nil {
				t.Fatalf("Parsea: %v", err)
			}
			if len(res.Movimientos) == 0 {
				t.Errorf("0 movimientos parseados")
			}
			t.Logf("banco=%-12s iban=%-24q moneda=%-3s movimientos=%d",
				res.Banco, res.IBAN, res.MonedaArchivo, len(res.Movimientos))
		})
	}
}
