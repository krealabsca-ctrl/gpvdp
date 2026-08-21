package bancos

import (
	"testing"
	"time"
)

func TestSiglaEmpresa(t *testing.T) {
	casos := []struct{ nombre, quiere string }{
		{"Valle de Paz", "VDP"},
		{"VALLE DE PAZ S.A.", "VDP"}, // aguanta mayúsculas y tipo legal
		{"Coopeprofa", "CPF"},
		{"Memorial Pets", "MPTS"},
		{"Memorial Pets Costa Rica", "MPTS"}, // gana la sigla oficial, no las iniciales
		// Empresa que todavía no tiene sigla definida: iniciales, no un nombre inventado.
		{"Servicios Funerarios del Este", "SFDE"},
		{"Nueva", "N"},
		{"", "EMPRESA"},
	}
	for _, c := range casos {
		if got := SiglaEmpresa(c.nombre); got != c.quiere {
			t.Errorf("SiglaEmpresa(%q) = %q, quiere %q", c.nombre, got, c.quiere)
		}
	}
}

func TestNombreArchivoReporte(t *testing.T) {
	// 17 de agosto de 2026 → 17082026, el formato que usa el usuario para archivar.
	en := time.Date(2026, 8, 17, 20, 39, 0, 0, time.UTC)
	casos := []struct{ empresa, detalle, quiere string }{
		{"Valle de Paz", "", "VDP 17082026.xlsx"},
		{"Valle de Paz", "corrido", "VDP 17082026 corrido.xlsx"},
		{"Coopeprofa", "cuadre", "CPF 17082026 cuadre.xlsx"},
		{"Memorial Pets", "", "MPTS 17082026.xlsx"},
		{"Valle de Paz", "   ", "VDP 17082026.xlsx"}, // detalle en blanco no deja espacio colgando
	}
	for _, c := range casos {
		if got := NombreArchivoReporte(c.empresa, en, c.detalle); got != c.quiere {
			t.Errorf("NombreArchivoReporte(%q, %q) = %q, quiere %q", c.empresa, c.detalle, got, c.quiere)
		}
	}
}
