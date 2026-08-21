package cxp

import "github.com/shopspring/decimal"

// Umbrales de aprobación por monto (CRC), confirmados por el Director Financiero:
//
//	≤ ₡1.000.000        → 1 aprobador
//	₡1.000.001–₡5.000.000 → 2 aprobadores
//	> ₡5.000.000        → 2 aprobadores, uno de ellos Gerencia General (mancomunada)
var (
	umbralUnAprobador  = decimal.NewFromInt(1_000_000)
	umbralDosAprobador = decimal.NewFromInt(5_000_000)
)

// requisitoAprobacion devuelve cuántas aprobaciones se requieren y si una debe ser de Gerencia.
func requisitoAprobacion(totalCRC decimal.Decimal) (requeridos int, requiereGerencia bool) {
	switch {
	case totalCRC.LessThanOrEqual(umbralUnAprobador):
		return 1, false
	case totalCRC.LessThanOrEqual(umbralDosAprobador):
		return 2, false
	default:
		return 2, true
	}
}

// rolPuedeAprobar indica si un rol está habilitado para aprobar documentos CxP.
// SUPUESTO (confirmar con el DF): aprueban Supervisor/Director Financiero, Gerencia General y Admin.
func rolPuedeAprobar(rol string) bool {
	switch rol {
	case "ADMIN", "GERENCIA_GENERAL", "DIRECTOR_FINANCIERO", "SUPERVISOR_FINANCIERO":
		return true
	default:
		return false
	}
}

func esRolGerencia(rol string) bool {
	return rol == "GERENCIA_GENERAL" || rol == "ADMIN"
}

// aprobacionCompleta indica si las aprobaciones acumuladas satisfacen el requisito del monto.
func aprobacionCompleta(totalCRC decimal.Decimal, rolesAprobadores []string) bool {
	requeridos, requiereGerencia := requisitoAprobacion(totalCRC)
	if len(rolesAprobadores) < requeridos {
		return false
	}
	if requiereGerencia {
		for _, r := range rolesAprobadores {
			if esRolGerencia(r) {
				return true
			}
		}
		return false
	}
	return true
}
