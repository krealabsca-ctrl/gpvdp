package bancos

// Sincronización del tipo de cambio con el BCCR (§22/§23). Reglas:
//  - Respeta el override MANUAL: si la cotización de esa fecha ya existe con
//    fuente=MANUAL, NO se pisa (el usuario manda sobre el BCCR).
//  - Es tolerante a fallo: todo intento (éxito o error) se registra en bccr_sync_log;
//    ante fallo NO se rompe nada (fallback = seguir con el último valor manual/BCCR).
//  - Al escribir una cotización BCCR, reusa RegistrarCotizacion (recalcula el
//    provisional escalonado del mes igual que la carga manual).

import (
	"context"
	"time"
)

// ResultadoSync resume el resultado de un intento de sincronización.
type ResultadoSync struct {
	Fecha     string `json:"fecha"`
	Indicador string `json:"indicador"`
	Valor     string `json:"valor"`
	Exito     bool   `json:"exito"`
	Omitido   bool   `json:"omitido"` // true si ya había un override manual y no se pisó
	Mensaje   string `json:"mensaje"`
}

// SincronizarBCCR obtiene del BCCR la cotización de `fecha` y la registra para la empresa.
// El override MANUAL nunca se pisa: además del pre-chequeo (mensaje claro), el upsert
// lleva un WHERE condicional que elimina la carrera check-then-write. Ante cualquier
// error de lectura se FALLA CERRADO (no se escribe nada).
func (s *Service) SincronizarBCCR(ctx context.Context, empresaID, fecha string, usuarioID string) (ResultadoSync, error) {
	if s.bccr == nil {
		return ResultadoSync{}, ErrBCCRNoConfigurado
	}
	t, err := time.Parse("2006-01-02", fecha)
	if err != nil {
		return ResultadoSync{}, ErrFechaInvalida
	}
	res := ResultadoSync{Fecha: fecha, Indicador: s.bccr.Indicador()}

	// Respetar override manual. Fail-closed: si no se puede LEER, no se escribe.
	_, fuente, existe, err := s.repo.CotizacionExistente(ctx, empresaID, fecha)
	if err != nil {
		res.Exito, res.Mensaje = false, "no se pudo verificar la cotización existente; no se escribió nada: "+err.Error()
		_ = s.repo.RegistrarSyncBCCR(ctx, logDe(empresaID, res))
		return res, nil
	}
	if existe && fuente == "MANUAL" {
		res.Exito, res.Omitido, res.Mensaje = true, true, "Se conservó el valor manual de esa fecha (no se sobrescribe con BCCR)."
		_ = s.repo.RegistrarSyncBCCR(ctx, logDe(empresaID, res))
		return res, nil
	}

	valor, err := s.bccr.Obtener(ctx, t)
	if err != nil {
		res.Exito, res.Mensaje = false, err.Error()
		_ = s.repo.RegistrarSyncBCCR(ctx, logDe(empresaID, res))
		return res, nil // fallback tolerante: no propaga el error como 500
	}
	escrito, err := s.repo.UpsertCotizacionBCCR(ctx, empresaID, fecha, valor)
	if err != nil {
		res.Exito, res.Mensaje = false, "obtenido del BCCR pero no se pudo guardar: "+err.Error()
		_ = s.repo.RegistrarSyncBCCR(ctx, logDe(empresaID, res))
		return res, nil
	}
	if !escrito {
		// Carrera: apareció un override MANUAL entre el chequeo y la escritura.
		res.Exito, res.Omitido, res.Mensaje = true, true, "Se conservó el valor manual de esa fecha (no se sobrescribe con BCCR)."
		_ = s.repo.RegistrarSyncBCCR(ctx, logDe(empresaID, res))
		return res, nil
	}
	// Igual que la carga manual: recalcular el provisional del mes si no está congelado.
	if anio, mes, ok := anioMes(fecha); ok {
		if estado, _, err := s.repo.EstadoTCMes(ctx, empresaID, anio, mes); err == nil && estado != "CONGELADO" {
			s.recalcularProvisional(ctx, empresaID, anio, mes)
		}
	}
	res.Exito, res.Valor, res.Mensaje = true, valor.String(), "Cotización sincronizada desde el BCCR."
	_ = s.repo.RegistrarSyncBCCR(ctx, logDe(empresaID, res))
	return res, nil
}

func logDe(empresaID string, r ResultadoSync) BCCRSyncLog {
	return BCCRSyncLog{
		EmpresaID: empresaID, Fecha: r.Fecha, Indicador: r.Indicador,
		Valor: r.Valor, Exito: r.Exito, Mensaje: r.Mensaje,
	}
}

// UltimoSyncBCCR devuelve el último intento de sincronización de la empresa (o nil).
func (s *Service) UltimoSyncBCCR(ctx context.Context, empresaID string) (*BCCRSyncLog, error) {
	return s.repo.UltimoSyncBCCR(ctx, empresaID)
}

// SyncProgramado corre el sync para TODAS las empresas activas para la fecha dada.
// Lo invoca el scheduler los días 1/15/último. Best-effort: un fallo por empresa
// no detiene a las demás (cada intento queda en la bitácora). Devuelve true solo
// si TODAS las empresas sincronizaron (u omitieron por override manual); con false
// el scheduler reintenta en el próximo tick (idempotente: MANUAL nunca se pisa).
func (s *Service) SyncProgramado(ctx context.Context, fecha string) bool {
	empresas, err := s.repo.EmpresasActivas(ctx)
	if err != nil {
		s.log.Warn("bccr: no se pudieron listar empresas para el sync programado")
		return false
	}
	todoOK := true
	for _, empresaID := range empresas {
		res, err := s.SincronizarBCCR(ctx, empresaID, fecha, "")
		if err != nil || !res.Exito {
			todoOK = false
			s.log.Warn("bccr: sync programado falló para una empresa")
		}
	}
	return todoOK
}

// EsDiaDeSync indica si `t` es día 1, 15 o el último del mes (los días de captura RN-10).
func EsDiaDeSync(t time.Time) bool {
	d := t.Day()
	ultimo := time.Date(t.Year(), t.Month()+1, 0, 0, 0, 0, 0, t.Location()).Day()
	return d == 1 || d == 15 || d == ultimo
}
