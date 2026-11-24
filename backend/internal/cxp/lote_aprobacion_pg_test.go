package cxp

// La aprobación del corte, contra PostgreSQL de verdad (mig 0123, 13-nov-2026).
//
// LO QUE SE PROTEGE ES QUE EL ARCHIVO DEL BANCO NO SALGA ANTES DE LA FIRMA. Todo lo demás de este
// flujo —los estados, los nombres, los motivos— es ceremonia si la macro se puede bajar igual de
// un corte que nadie aprobó. Esa era exactamente la situación antes de esta migración, y es un
// defecto que no da ningún error: el archivo se baja, se sube al banco y la plata sale.
//
// Y lo segundo: QUE EXCLUIR NO SEA DENEGAR. Una factura que la gerencia saca del corte tiene que
// volver a la bandeja entera —en PROGRAMADO, con su aprobación y su validación de área— o el
// próximo corte la deja afuera y alguien la vuelve a tramitar desde cero. Ese es el reproceso que
// esto viene a evitar, así que es lo que hay que medir.

import (
	"context"
	"errors"
	"strings"
	"testing"

	"go.uber.org/zap"
)

// corteConFactura arma un corte con una factura PROGRAMADA adentro.
func corteConFactura(t *testing.T, r *pgRepository) (loteID, docID string) {
	t.Helper()
	ctx := context.Background()
	if err := r.pool.QueryRow(ctx, `
		INSERT INTO lote_pago (empresa_id, numero, fecha_corte, estado, creado_por)
		VALUES ($1::uuid, (SELECT COALESCE(MAX(numero), 0) + 1 FROM lote_pago), '2026-11-13',
		        'ABIERTO', $2::uuid)
		RETURNING id::text`, empresaLote, usuarioLote).Scan(&loteID); err != nil {
		t.Fatalf("sembrar corte: %v", err)
	}
	if err := r.pool.QueryRow(ctx, `
		INSERT INTO documento_cxp
			(empresa_id, proveedor_id, clave, consecutivo, fecha_emision, estado, moneda,
			 total, total_crc, retencion, lote_id)
		VALUES ($1::uuid, $2::uuid, 'c-' || gen_random_uuid(), '001', '2026-11-01', 'PROGRAMADO',
		        'CRC', 100000, 100000, 0, $3::uuid)
		RETURNING id::text`, empresaLote, provLote, loteID).Scan(&docID); err != nil {
		t.Fatalf("sembrar factura del corte: %v", err)
	}
	return loteID, docID
}

func servicioDeLotes(t *testing.T, r *pgRepository) *Service {
	t.Helper()
	return NewService(r, nil, zap.NewNop())
}

// EL ARCHIVO DEL BANCO NO SALE ANTES DE LA APROBACIÓN.
func TestLaMacroNoSaleDeUnCorteSinAprobar(t *testing.T) {
	repo := baseDeLotes(t)
	svc := servicioDeLotes(t, repo)
	ctx := context.Background()
	loteID, _ := corteConFactura(t, repo)

	// ABIERTO: es como estaban los 40 lotes existentes, y es de donde se bajaba la macro antes.
	if _, _, err := svc.MacroLote(ctx, empresaLote, loteID); !errors.Is(err, ErrLoteSinAprobar) {
		t.Fatalf("con el corte ABIERTO la macro dio %v; tenía que frenarse: así es como se subía "+
			"al banco un corte que nadie aprobó", err)
	}

	// EN_APROBACION tampoco: la gerencia lo está mirando, todavía no dijo que sí.
	if _, err := svc.MandarLoteAAprobar(ctx, empresaLote, loteID, usuarioLote); err != nil {
		t.Fatalf("mandar a aprobar: %v", err)
	}
	if _, _, err := svc.MacroLote(ctx, empresaLote, loteID); !errors.Is(err, ErrLoteSinAprobar) {
		t.Errorf("con el corte EN APROBACIÓN la macro dio %v; tenía que frenarse", err)
	}

	// APROBADO sí.
	if _, err := svc.AprobarLote(ctx, empresaLote, loteID, usuarioLote); err != nil {
		t.Fatalf("aprobar: %v", err)
	}
	if _, _, err := svc.MacroLote(ctx, empresaLote, loteID); err != nil {
		t.Errorf("con el corte APROBADO la macro dio %v; tenía que salir", err)
	}
}

// EXCLUIR NO ES DENEGAR: la factura vuelve entera a la bandeja.
func TestExcluirDevuelveLaFacturaALaBandejaSinCastigarla(t *testing.T) {
	repo := baseDeLotes(t)
	svc := servicioDeLotes(t, repo)
	ctx := context.Background()
	loteID, docID := corteConFactura(t, repo)

	if _, err := svc.MandarLoteAAprobar(ctx, empresaLote, loteID, usuarioLote); err != nil {
		t.Fatalf("mandar a aprobar: %v", err)
	}
	ex, err := svc.ExcluirFacturaDelLote(ctx, empresaLote, loteID, docID, usuarioLote,
		"el proveedor no entregó la nota de crédito acordada")
	if err != nil {
		t.Fatalf("excluir: %v", err)
	}

	var estado, motivo string
	var loteSigue *string
	var numero *int64
	if err := repo.pool.QueryRow(ctx,
		`SELECT estado, lote_id::text, COALESCE(excluido_lote_motivo,''), excluido_lote_numero
		   FROM documento_cxp WHERE id = $1::uuid`, docID).
		Scan(&estado, &loteSigue, &motivo, &numero); err != nil {
		t.Fatalf("leer la factura: %v", err)
	}

	// LO QUE IMPORTA: sigue PROGRAMADA. No DENEGADA, no RECIBIDO, no nada que obligue a volver a
	// tramitarla. Entra al próximo corte tal como está.
	if estado != "PROGRAMADO" {
		t.Errorf("la factura quedó en %q; excluirla de un corte no la castiga, la posterga", estado)
	}
	if loteSigue != nil {
		t.Error("la factura sigue colgando del corte: no salió")
	}
	if motivo == "" {
		t.Error("se perdió el motivo: quien la vuelva a cortar la semana entrante no sabe por qué salió")
	}
	if numero == nil || *numero == 0 {
		t.Error("no quedó de qué corte salió: la factura no puede explicar por qué no se pagó")
	}
	if ex.Motivo == "" || ex.Proveedor == "" {
		t.Errorf("la respuesta llegó incompleta: %+v", ex)
	}

	// Y el corte la lista como excluida, que es lo que se compara después contra lo que se pagó.
	lista, err := svc.ExcluidasDelLote(ctx, empresaLote, loteID)
	if err != nil {
		t.Fatalf("excluidas: %v", err)
	}
	if len(lista) != 1 || lista[0].DocumentoID != docID {
		t.Errorf("el corte lista %d excluida(s); se esperaba la que salió", len(lista))
	}
}

// Un corte APROBADO ya no se toca: la firma describe un conjunto y cambiarlo la vuelve mentira.
func TestNoSeSacaUnaFacturaDeUnCorteYaAprobado(t *testing.T) {
	repo := baseDeLotes(t)
	svc := servicioDeLotes(t, repo)
	ctx := context.Background()
	loteID, docID := corteConFactura(t, repo)

	if _, err := svc.MandarLoteAAprobar(ctx, empresaLote, loteID, usuarioLote); err != nil {
		t.Fatalf("mandar a aprobar: %v", err)
	}
	if _, err := svc.AprobarLote(ctx, empresaLote, loteID, usuarioLote); err != nil {
		t.Fatalf("aprobar: %v", err)
	}
	_, err := svc.ExcluirFacturaDelLote(ctx, empresaLote, loteID, docID, usuarioLote,
		"se quiso sacar después de que la gerencia firmara")
	if !errors.Is(err, ErrLoteNoSeTocaAprobado) {
		t.Errorf("err = %v; se esperaba la guarda del corte aprobado", err)
	}
}

// Devolver manda el corte ENTERO de vuelta, con su motivo, y deja aprobar otra vez después.
func TestDevolverElCorteLoRegresaAbiertoYDejaRehacerlo(t *testing.T) {
	repo := baseDeLotes(t)
	svc := servicioDeLotes(t, repo)
	ctx := context.Background()
	loteID, _ := corteConFactura(t, repo)

	if _, err := svc.MandarLoteAAprobar(ctx, empresaLote, loteID, usuarioLote); err != nil {
		t.Fatalf("mandar a aprobar: %v", err)
	}
	lote, err := svc.DevolverLote(ctx, empresaLote, loteID, usuarioLote,
		"la fecha de corte es del viernes pasado, rehacelo con la de esta semana")
	if err != nil {
		t.Fatalf("devolver: %v", err)
	}
	if lote.Estado != LoteAbierto {
		t.Errorf("estado = %q; devolver lo deja donde tesorería puede trabajarlo", lote.Estado)
	}
	if lote.DevolucionMotivo == "" || lote.DevueltoPor == "" {
		t.Errorf("se perdió quién lo devolvió o por qué: %+v", lote)
	}

	// Y el ciclo se puede repetir: se corrige y se manda de nuevo.
	if _, err := svc.MandarLoteAAprobar(ctx, empresaLote, loteID, usuarioLote); err != nil {
		t.Fatalf("volver a mandar a aprobar: %v", err)
	}
	aprobado, err := svc.AprobarLote(ctx, empresaLote, loteID, usuarioLote)
	if err != nil {
		t.Fatalf("aprobar después de devolver: %v", err)
	}
	if aprobado.AprobadoPor == "" {
		t.Error("el corte aprobado no dice quién lo aprobó: no sirve de respaldo")
	}
	// La devolución NO se borra al aprobar: que el corte haya ido y vuelto es parte del expediente.
	if aprobado.DevueltoEn == "" {
		t.Error("se borró la devolución al aprobar; el corte ya no puede contar que fue devuelto")
	}
}

// Cada paso solo sale desde su estado: aprobar lo que nadie mandó deja una firma sobre un corte
// que la gerencia nunca vio.
func TestCadaPasoExigeElEstadoQueLeCorresponde(t *testing.T) {
	repo := baseDeLotes(t)
	svc := servicioDeLotes(t, repo)
	ctx := context.Background()
	loteID, _ := corteConFactura(t, repo)

	if _, err := svc.AprobarLote(ctx, empresaLote, loteID, usuarioLote); !errors.Is(err, ErrLoteNoEstaEnAprobacion) {
		t.Errorf("aprobar un corte ABIERTO dio %v; nadie lo mandó a aprobar", err)
	}
	if _, err := svc.DevolverLote(ctx, empresaLote, loteID, usuarioLote,
		"devolver algo que nadie mandó a aprobar"); !errors.Is(err, ErrLoteNoEstaEnAprobacion) {
		t.Errorf("devolver un corte ABIERTO dio %v", err)
	}
	if _, err := svc.MandarLoteAAprobar(ctx, empresaLote, loteID, usuarioLote); err != nil {
		t.Fatalf("mandar a aprobar: %v", err)
	}
	if _, err := svc.MandarLoteAAprobar(ctx, empresaLote, loteID, usuarioLote); !errors.Is(err, ErrLoteNoEstaAbierto) {
		t.Errorf("mandar dos veces dio %v; el segundo tenía que frenarse", err)
	}
}

// El motivo reflejo no alcanza, ni para devolver ni para excluir.
func TestLosMotivosDeLaAprobacionExigenSustancia(t *testing.T) {
	repo := baseDeLotes(t)
	svc := servicioDeLotes(t, repo)
	ctx := context.Background()
	loteID, docID := corteConFactura(t, repo)
	if _, err := svc.MandarLoteAAprobar(ctx, empresaLote, loteID, usuarioLote); err != nil {
		t.Fatalf("mandar a aprobar: %v", err)
	}

	if _, err := svc.DevolverLote(ctx, empresaLote, loteID, usuarioLote, "no"); !errors.Is(err, ErrMotivoDevolucionRequerido) {
		t.Errorf("devolver con «no» dio %v", err)
	}
	if _, err := svc.ExcluirFacturaDelLote(ctx, empresaLote, loteID, docID, usuarioLote, "ok"); !errors.Is(err, ErrMotivoExclusionRequerido) {
		t.Errorf("excluir con «ok» dio %v", err)
	}
	// Y el corte sigue donde estaba: un motivo rechazado no puede haber movido nada.
	estado, _, err := repo.EstadoDelLote(ctx, empresaLote, loteID)
	if err != nil {
		t.Fatalf("estado: %v", err)
	}
	if estado != LoteEnAprobacion {
		t.Errorf("el corte quedó en %q después de dos intentos rechazados", estado)
	}
}

// El corte de OTRA empresa no se aprueba desde acá.
func TestLaAprobacionNoCruzaEmpresas(t *testing.T) {
	repo := baseDeLotes(t)
	svc := servicioDeLotes(t, repo)
	ctx := context.Background()
	loteID, docID := corteConFactura(t, repo)

	if _, err := svc.MandarLoteAAprobar(ctx, otraEmpLote, loteID, usuarioLote); !errors.Is(err, ErrLoteNoEncontrado) {
		t.Errorf("otra empresa mandó a aprobar este corte: %v", err)
	}
	if _, err := svc.MandarLoteAAprobar(ctx, empresaLote, loteID, usuarioLote); err != nil {
		t.Fatalf("mandar a aprobar: %v", err)
	}
	if _, err := svc.AprobarLote(ctx, otraEmpLote, loteID, usuarioLote); !errors.Is(err, ErrLoteNoEncontrado) {
		t.Errorf("otra empresa aprobó este corte: %v", err)
	}
	_, err := svc.ExcluirFacturaDelLote(ctx, otraEmpLote, loteID, docID, usuarioLote,
		"sacar una factura de un corte de otra empresa")
	if !errors.Is(err, ErrLoteNoEncontrado) {
		t.Errorf("otra empresa sacó una factura de este corte: %v", err)
	}
	// Y la factura no se movió.
	var lote *string
	if err := repo.pool.QueryRow(ctx,
		`SELECT lote_id::text FROM documento_cxp WHERE id = $1::uuid`, docID).Scan(&lote); err != nil {
		t.Fatalf("leer la factura: %v", err)
	}
	if lote == nil {
		t.Error("la factura salió del corte por una petición de otra empresa")
	}
}

// EL FILTRO POR EMPRESA VIVE EN LA CONSULTA, no solo en el servicio.
//
// Esta prueba existe por una falsificación que NO falló: al quitarle el `empresa_id` al UPDATE de
// `ExcluirDelLote`, la prueba de multiempresa de arriba siguió pasando, porque el servicio
// consulta el estado del corte primero y corta ahí. La defensa estaba en una sola capa.
//
// Un repositorio que no filtra por empresa es un defecto de seguridad aunque hoy nadie lo llame
// mal: el día que alguien reordene el servicio —o llame a la consulta desde otro lado— el dato de
// una empresa sale por la puerta de otra, y no hay error que lo delate.
func TestLasConsultasDeLaAprobacionFiltranPorEmpresaEllasMismas(t *testing.T) {
	repo := baseDeLotes(t)
	ctx := context.Background()
	loteID, docID := corteConFactura(t, repo)

	// Cada una se llama DIRECTO con la empresa equivocada: ninguna puede mover una fila.
	if n, err := repo.MandarAAprobar(ctx, otraEmpLote, loteID, usuarioLote); err != nil || n != 0 {
		t.Errorf("MandarAAprobar movió %d fila(s) con otra empresa (err=%v)", n, err)
	}
	if n, err := repo.AprobarLote(ctx, otraEmpLote, loteID, usuarioLote); err != nil || n != 0 {
		t.Errorf("AprobarLote movió %d fila(s) con otra empresa (err=%v)", n, err)
	}
	if n, err := repo.DevolverLote(ctx, otraEmpLote, loteID, usuarioLote, "x"); err != nil || n != 0 {
		t.Errorf("DevolverLote movió %d fila(s) con otra empresa (err=%v)", n, err)
	}
	if n, err := repo.ExcluirDelLote(ctx, otraEmpLote, loteID, docID, usuarioLote, "x"); err != nil || n != 0 {
		t.Errorf("ExcluirDelLote movió %d fila(s) con otra empresa (err=%v): la factura salió del "+
			"corte por una petición ajena", n, err)
	}
	if _, existe, err := repo.EstadoDelLote(ctx, otraEmpLote, loteID); err != nil || existe {
		t.Errorf("EstadoDelLote encontró el corte desde otra empresa (existe=%v, err=%v)", existe, err)
	}
	if lista, err := repo.ExcluidasDelLote(ctx, otraEmpLote, loteID); err != nil || len(lista) != 0 {
		t.Errorf("ExcluidasDelLote devolvió %d fila(s) desde otra empresa (err=%v)", len(lista), err)
	}

	// Y el corte quedó intacto: ninguna de las seis llamadas lo tocó.
	estado, _, err := repo.EstadoDelLote(ctx, empresaLote, loteID)
	if err != nil {
		t.Fatalf("estado: %v", err)
	}
	if estado != LoteAbierto {
		t.Errorf("el corte quedó en %q después de seis llamadas de otra empresa", estado)
	}
}

// Los mensajes de error dicen QUÉ HACER, no solo que algo falló.
func TestLosErroresDeLaAprobacionDicenQueHacer(t *testing.T) {
	for _, c := range []struct {
		err   error
		pista string
	}{
		{ErrLoteSinAprobar, "aprobado"},
		{ErrLoteNoSeTocaAprobado, "devolverlo"},
		{ErrMotivoDevolucionRequerido, "corregir"},
		{ErrMotivoExclusionRequerido, "por qué"},
	} {
		if !strings.Contains(c.err.Error(), c.pista) {
			t.Errorf("%q no dice qué hacer (se buscaba %q)", c.err.Error(), c.pista)
		}
	}
}
