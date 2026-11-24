package bancos

// Enlazar dos movimientos que el banco reemitió, contra PostgreSQL de verdad (10-nov-2026).
//
// «Hay un banco que el 100% cambia una transacción y no hay forma de mapearla… una vez que yo los
// identifique me permita relacionarlos como el mismo movimiento de forma manual.»
//
// El caso medido: un crédito de ₡880.000 entra el 4 de noviembre y se clasifica; el 5 vuelve a
// entrar con el MISMO consecutivo, el mismo monto y otra descripción, y cae en «por clasificar».
// El detector de duplicados no lo ve —busca el mismo día con consecutivos distintos, lo contrario—
// y ninguno de los dos se puede dejar: el monto está dos veces en los libros.
//
// Lo que hay que proteger acá no lo puede demostrar un doble en memoria:
//
//  1. Que enlazar sea ATÓMICO. Escribir el enlace sin sacar al otro de los libros deja el monto
//     contado dos veces, y nada lo delata: las dos filas son legítimas por separado.
//  2. Que NO se pueda enlazar lo que tiene algo colgando ni lo que está en un mes cerrado.
//  3. Que la sugerencia no proponga RECURRENCIAS. Medido sobre los datos reales, la huella sin
//     límite de días devuelve sobre todo pagos repetidos legítimos —un ahorro de ₡1.000.000 que el
//     BCR debita cada semana con el mismo consecutivo—, y enlazarlos fusionaría nueve débitos de un
//     millón en uno solo.

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// sembrarClasificado le pone concepto y clasificación a un movimiento y lo marca revisado, como
// queda el que alguien ya trabajó.
func sembrarClasificado(t *testing.T, r *pgRepository, movID string) (string, string) {
	t.Helper()
	ctx := context.Background()
	var conceptoID, clasifID string
	if err := r.pool.QueryRow(ctx, `
		INSERT INTO concepto (empresa_id, nombre) VALUES ($1::uuid, 'Ingresos')
		RETURNING id::text`, empExc).Scan(&conceptoID); err != nil {
		t.Fatalf("sembrar concepto: %v", err)
	}
	if err := r.pool.QueryRow(ctx, `
		INSERT INTO clasificacion (empresa_id, concepto_id, nombre)
		VALUES ($1::uuid, $2::uuid, 'Servicios de Emergencias') RETURNING id::text`,
		empExc, conceptoID).Scan(&clasifID); err != nil {
		t.Fatalf("sembrar clasificación: %v", err)
	}
	if _, err := r.pool.Exec(ctx, `
		UPDATE movimiento_bancario
		   SET concepto_id = $2::uuid, clasificacion_id = $3::uuid, estado_clasificacion = 'REVISADO'
		 WHERE id = $1::uuid`, movID, conceptoID, clasifID); err != nil {
		t.Fatalf("clasificar movimiento: %v", err)
	}
	return conceptoID, clasifID
}

func clasificacionDe(t *testing.T, r *pgRepository, movID string) (string, string) {
	t.Helper()
	var clasif, estado *string
	if err := r.pool.QueryRow(context.Background(),
		`SELECT clasificacion_id::text, estado_clasificacion FROM movimiento_bancario WHERE id = $1::uuid`,
		movID).Scan(&clasif, &estado); err != nil {
		t.Fatalf("leer clasificación: %v", err)
	}
	c, e := "", ""
	if clasif != nil {
		c = *clasif
	}
	if estado != nil {
		e = *estado
	}
	return c, e
}

// El camino completo: el reemitido se queda, el viejo sale, y la clasificación viaja sola.
func TestEnlazarUnaReemisionDejaUnSoloMovimientoEnLosLibros(t *testing.T) {
	repo := baseDeExclusion(t)
	svc := &Service{repo: repo}
	ctx := context.Background()

	viejo := sembrarMov(t, repo, "2026-11-04", "406802824", "TEF A : 903825354", 880000)
	_, clasifEsperada := sembrarClasificado(t, repo, viejo)
	nuevo := sembrarMov(t, repo, "2026-11-05", "406802824", "TEF DE:HILARY AILEEN PINTO CUB", 880000)

	if err := svc.EnlazarReemision(ctx, empExc, nuevo, viejo,
		"El BAC reemitió el mismo crédito al día siguiente", usrExc); err != nil {
		t.Fatalf("enlazar: %v", err)
	}

	// EL MONTO QUEDA UNA SOLA VEZ. Es lo único que de verdad importa: si los dos siguieran en los
	// libros, la cartera tendría ₡1.760.000 donde el banco movió ₡880.000.
	if !estaIncluido(t, repo, nuevo) {
		t.Error("el movimiento reemitido debería quedar en los libros")
	}
	if estaIncluido(t, repo, viejo) {
		t.Error("el movimiento reemplazado debería haber salido de los libros")
	}

	// La clasificación viaja al que queda: es lo que ahorra volver a clasificarlo a mano.
	clasif, estado := clasificacionDe(t, repo, nuevo)
	if clasif != clasifEsperada {
		t.Errorf("clasificación del vigente = %q, se esperaba %q (no heredó)", clasif, clasifEsperada)
	}
	if estado != "REVISADO" {
		t.Errorf("estado = %q, se esperaba REVISADO", estado)
	}

	// El que salió sigue existiendo con su descripción original: es evidencia del banco, no se toca.
	var desc string
	if err := repo.pool.QueryRow(ctx,
		`SELECT descripcion FROM movimiento_bancario WHERE id = $1::uuid`, viejo).Scan(&desc); err != nil {
		t.Fatalf("leer el reemplazado: %v", err)
	}
	if desc != "TEF A : 903825354" {
		t.Errorf("descripción del reemplazado = %q: no se debe sobrescribir", desc)
	}

	// Y la lista lo cuenta como un enlace, con el motivo de quien lo decidió.
	lista, err := svc.Reemisiones(ctx, empExc)
	if err != nil {
		t.Fatalf("listar reemisiones: %v", err)
	}
	if len(lista) != 1 {
		t.Fatalf("reemisiones = %d, se esperaba 1", len(lista))
	}
	if lista[0].Vigente.ID != nuevo || lista[0].Reemplazado.ID != viejo {
		t.Error("la lista invirtió cuál queda y cuál salió")
	}
	if !lista[0].MismaClasificacion {
		t.Error("la lista debería decir que los dos quedaron con la misma clasificación")
	}
}

// Deshacer devuelve el movimiento a los libros. Sin esto, un enlace equivocado sería permanente.
func TestDeshacerUnaReemisionDevuelveElMovimientoALosLibros(t *testing.T) {
	repo := baseDeExclusion(t)
	svc := &Service{repo: repo}
	ctx := context.Background()

	viejo := sembrarMov(t, repo, "2026-11-04", "406802824", "TEF A : 903825354", 880000)
	nuevo := sembrarMov(t, repo, "2026-11-05", "406802824", "TEF DE:HILARY", 880000)
	if err := svc.EnlazarReemision(ctx, empExc, nuevo, viejo,
		"Reemisión del BAC del día siguiente", usrExc); err != nil {
		t.Fatalf("enlazar: %v", err)
	}

	if err := svc.DeshacerReemision(ctx, empExc, nuevo, usrExc); err != nil {
		t.Fatalf("deshacer: %v", err)
	}
	if !estaIncluido(t, repo, viejo) {
		t.Error("deshacer debería devolver el reemplazado a los libros")
	}
	lista, err := svc.Reemisiones(ctx, empExc)
	if err != nil {
		t.Fatalf("listar: %v", err)
	}
	if len(lista) != 0 {
		t.Errorf("reemisiones = %d tras deshacer, se esperaba 0", len(lista))
	}
	// Y se puede volver a enlazar: deshacer deja todo como antes, no en un estado intermedio.
	if err := svc.EnlazarReemision(ctx, empExc, nuevo, viejo,
		"Reemisión del BAC, segundo intento", usrExc); err != nil {
		t.Errorf("no se pudo volver a enlazar tras deshacer: %v", err)
	}
}

// Una clasificación puesta a mano NO se pisa. Heredar de más destruye trabajo sin avisar.
func TestNoPisaLaClasificacionDelQueYaEstabaClasificado(t *testing.T) {
	repo := baseDeExclusion(t)
	svc := &Service{repo: repo}
	ctx := context.Background()

	viejo := sembrarMov(t, repo, "2026-11-04", "406802824", "TEF A", 880000)
	sembrarClasificado(t, repo, viejo)
	nuevo := sembrarMov(t, repo, "2026-11-05", "406802824", "TEF DE", 880000)
	// El reemitido YA fue clasificado por alguien, con otra cosa.
	var otroConcepto, otraClasif string
	if err := repo.pool.QueryRow(ctx, `
		INSERT INTO concepto (empresa_id, nombre) VALUES ($1::uuid, 'Otros ingresos')
		RETURNING id::text`, empExc).Scan(&otroConcepto); err != nil {
		t.Fatalf("sembrar concepto: %v", err)
	}
	if err := repo.pool.QueryRow(ctx, `
		INSERT INTO clasificacion (empresa_id, concepto_id, nombre)
		VALUES ($1::uuid, $2::uuid, 'Donaciones') RETURNING id::text`,
		empExc, otroConcepto).Scan(&otraClasif); err != nil {
		t.Fatalf("sembrar clasificación: %v", err)
	}
	if _, err := repo.pool.Exec(ctx, `
		UPDATE movimiento_bancario SET concepto_id = $2::uuid, clasificacion_id = $3::uuid,
		       estado_clasificacion = 'REVISADO' WHERE id = $1::uuid`,
		nuevo, otroConcepto, otraClasif); err != nil {
		t.Fatalf("clasificar el nuevo: %v", err)
	}

	if err := svc.EnlazarReemision(ctx, empExc, nuevo, viejo,
		"Reemisión del BAC con los dos clasificados", usrExc); err != nil {
		t.Fatalf("enlazar: %v", err)
	}
	clasif, _ := clasificacionDe(t, repo, nuevo)
	if clasif != otraClasif {
		t.Errorf("clasificación = %q, se esperaba %q: enlazar pisó una clasificación puesta a mano",
			clasif, otraClasif)
	}
}

// El freno que pidió el usuario: si algo cuelga del que saldría, se frena Y se dice qué deshacer.
func TestNoEnlazaSiAlgoCuelgaDelMovimientoQueSaldria(t *testing.T) {
	repo := baseDeExclusion(t)
	svc := &Service{repo: repo}
	ctx := context.Background()

	viejo := sembrarMov(t, repo, "2026-11-04", "406802824", "TEF A", 880000)
	nuevo := sembrarMov(t, repo, "2026-11-05", "406802824", "TEF DE", 880000)
	// Un cobro de CxC aplicado contra el movimiento viejo: sacarlo de los libros dejaría ese cobro
	// apuntando a plata que ya no cuenta, y eso no se ve hasta el cierre.
	if _, err := repo.pool.Exec(ctx, `
		INSERT INTO cobro_cxc (empresa_id, movimiento_bancario_id, fecha_pago, monto)
		VALUES ($1::uuid, $2::uuid, '2026-11-04', 880000)`, empExc, viejo); err != nil {
		t.Fatalf("sembrar cobro de CxC: %v", err)
	}

	err := svc.EnlazarReemision(ctx, empExc, nuevo, viejo, "Reemisión del BAC", usrExc)
	if !errors.Is(err, ErrReemisionConDependencias) {
		t.Fatalf("error = %v, se esperaba ErrReemisionConDependencias", err)
	}
	// El mensaje tiene que NOMBRAR qué deshacer: «hay dependencias» a secas manda a buscar a ciegas.
	if !strings.Contains(err.Error(), "cobro de CxC") {
		t.Errorf("el error no dice qué cuelga: %q", err)
	}
	// Y nada se movió: el freno es antes de escribir, no a medias.
	if !estaIncluido(t, repo, viejo) {
		t.Error("el movimiento salió de los libros pese al freno")
	}
}

// Un mes cerrado es intocable: sus números ya se reportaron.
func TestNoEnlazaSiElQueSaleEstaEnUnPeriodoCerrado(t *testing.T) {
	repo := baseDeExclusion(t)
	svc := &Service{repo: repo}
	ctx := context.Background()

	viejo := sembrarMov(t, repo, "2026-10-30", "406802824", "TEF A", 880000)
	nuevo := sembrarMov(t, repo, "2026-11-02", "406802824", "TEF DE", 880000)
	if _, err := repo.pool.Exec(ctx, `
		INSERT INTO periodo_cierre (empresa_id, anio, mes, no_identificados_al_cierre)
		VALUES ($1::uuid, 2026, 10, 0)`, empExc); err != nil {
		t.Fatalf("cerrar período: %v", err)
	}

	err := svc.EnlazarReemision(ctx, empExc, nuevo, viejo, "Reemisión del BAC a fin de mes", usrExc)
	if !errors.Is(err, ErrReemisionPeriodoCerrado) {
		t.Fatalf("error = %v, se esperaba ErrReemisionPeriodoCerrado", err)
	}
	if !estaIncluido(t, repo, viejo) {
		t.Error("el movimiento de un mes cerrado salió de los libros")
	}
}

// La sugerencia no debe proponer recurrencias: es el defecto que habría fusionado nueve débitos de
// un millón en uno solo. Cubre las dos que existen en los datos reales, la mensual y la SEMANAL.
func TestLaSugerenciaNoProponeRecurrenciasMensuales(t *testing.T) {
	repo := baseDeExclusion(t)
	svc := &Service{repo: repo}
	ctx := context.Background()

	// Una reemisión de verdad: al día siguiente.
	sembrarMov(t, repo, "2026-11-04", "406802824", "TEF A : 903825354", 880000)
	sembrarMov(t, repo, "2026-11-05", "406802824", "TEF DE:HILARY", 880000)
	// Una cuota mensual legítima, con el MISMO consecutivo —así las manda el BN— y el mismo monto.
	sembrarMov(t, repo, "2026-09-01", "96483403", "ID 6-0103-0362 WILBURG ELIZONDO", 7800)
	sembrarMov(t, repo, "2026-10-01", "96483403", "ID 6-0103-0362 WILBURG ELIZONDO", 7800)
	// Y el ahorro SEMANAL del BCR, que es el que de verdad ensuciaba la lista: con la ventana en
	// siete días, siete pares de estos tapaban al único candidato real. La recurrencia más corta
	// que existe en los datos es esta, y por eso el tope quedó en tres días.
	sembrarMov(t, repo, "2026-08-21", "1521673", "DEB. X AHORRO PRO CC", 1000000)
	sembrarMov(t, repo, "2026-08-28", "1521673", "DEB. X AHORRO PRO CC", 1000000)

	items, err := svc.CandidatosReemision(ctx, empExc)
	if err != nil {
		t.Fatalf("candidatos: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("candidatos = %d, se esperaba 1 (la mensual no debe aparecer): %+v", len(items), items)
	}
	if items[0].DiasEntre != 1 {
		t.Errorf("días entre = %d, se esperaba 1", items[0].DiasEntre)
	}
	if items[0].Anterior.Fecha != "2026-11-04" || items[0].Posterior.Fecha != "2026-11-05" {
		t.Errorf("el par propuesto no es el reemitido: %s → %s",
			items[0].Anterior.Fecha, items[0].Posterior.Fecha)
	}
}

// Un par ya enlazado deja de proponerse: proponer lo resuelto manda a corregir dos veces.
func TestLoYaEnlazadoDejaDeProponerse(t *testing.T) {
	repo := baseDeExclusion(t)
	svc := &Service{repo: repo}
	ctx := context.Background()

	viejo := sembrarMov(t, repo, "2026-11-04", "406802824", "TEF A", 880000)
	nuevo := sembrarMov(t, repo, "2026-11-05", "406802824", "TEF DE", 880000)
	if items, err := svc.CandidatosReemision(ctx, empExc); err != nil || len(items) != 1 {
		t.Fatalf("antes de enlazar: candidatos = %d, err = %v (se esperaba 1)", len(items), err)
	}
	if err := svc.EnlazarReemision(ctx, empExc, nuevo, viejo, "Reemisión del BAC", usrExc); err != nil {
		t.Fatalf("enlazar: %v", err)
	}
	items, err := svc.CandidatosReemision(ctx, empExc)
	if err != nil {
		t.Fatalf("candidatos: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("candidatos = %d tras enlazar, se esperaba 0", len(items))
	}
}

// Las reglas que impiden enlazar cosas que no son el mismo hecho.
func TestNoEnlazaMovimientosQueNoSonElMismoHecho(t *testing.T) {
	repo := baseDeExclusion(t)
	svc := &Service{repo: repo}
	ctx := context.Background()

	base := sembrarMov(t, repo, "2026-11-05", "406802824", "TEF DE", 880000)
	otroMonto := sembrarMov(t, repo, "2026-11-04", "406802824", "TEF A", 500000)

	casos := []struct {
		nombre    string
		vigente   string
		saliente  string
		motivo    string
		esperado  error
		explicaQu string
	}{
		{"consigo mismo", base, base, "Un movimiento contra sí mismo",
			ErrReemisionMismoMovimiento, "enlazarse a sí mismo sacaría de los libros al que queda"},
		{"montos distintos", base, otroMonto, "Montos que no coinciden",
			ErrReemisionDistintoMonto, "una reemisión del banco conserva la cifra"},
		{"motivo corto", base, otroMonto, "corto",
			ErrMotivoReemisionRequerido, "sin motivo nadie puede auditar por qué falta esa plata"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			err := svc.EnlazarReemision(ctx, empExc, c.vigente, c.saliente, c.motivo, usrExc)
			if !errors.Is(err, c.esperado) {
				t.Fatalf("error = %v, se esperaba %v (%s)", err, c.esperado, c.explicaQu)
			}
			if !estaIncluido(t, repo, c.saliente) {
				t.Error("algo salió de los libros pese al rechazo")
			}
		})
	}
}
