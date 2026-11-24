package cxp

// Las firmas del papel de respaldo, contra PostgreSQL de verdad (11-nov-2026).
//
// El Excel que este papel reemplaza traía cuatro nombres TECLEADOS —«Solicitado por», «Revisado
// por», «Aprobado por», «Pagado por»— y por lo tanto podía afirmar cualquier cosa. Decisión del
// Director Financiero: salen del sistema, de quién hizo cada paso.
//
// Lo que se protege acá no lo puede demostrar un doble, porque es una consulta que cruza la
// auditoría con las facturas del corte:
//
//  1. QUE UN PASO QUE NADIE HIZO SALGA VACÍO, no con un nombre cualquiera. Es la diferencia entre
//     «esto no se aprobó» y una firma falsa en un papel de respaldo.
//  2. QUE APAREZCAN TODOS LOS QUE PARTICIPARON. Un corte de cien facturas pasa por varios
//     revisores; poner solo uno diría que el resto no revisó nada.
//  3. QUE NO SE CUELE LA AUDITORÍA DE OTRO CORTE NI DE OTRA EMPRESA.

import (
	"context"
	"strings"
	"testing"
)

// sembrarCorteConFactura crea un corte con una factura adentro y devuelve los dos ids.
func sembrarCorteConFactura(t *testing.T, r *pgRepository) (loteID, docID string) {
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
		VALUES ($1::uuid, $2::uuid, 'c-' || gen_random_uuid(), '001', '2026-11-01', 'APROBADO',
		        'CRC', 100000, 100000, 0, $3::uuid)
		RETURNING id::text`, empresaLote, provLote, loteID).Scan(&docID); err != nil {
		t.Fatalf("sembrar factura del corte: %v", err)
	}
	return loteID, docID
}

// sembrarUsuario crea una persona y devuelve su id, para que el papel tenga a quién nombrar.
func sembrarUsuario(t *testing.T, r *pgRepository, nombre, email string) string {
	t.Helper()
	var id string
	if err := r.pool.QueryRow(context.Background(), `
		INSERT INTO usuario (nombre, email, password_hash) VALUES ($1, $2, 'x')
		RETURNING id::text`, nombre, email).Scan(&id); err != nil {
		t.Fatalf("sembrar usuario: %v", err)
	}
	return id
}

// sembrarEvento escribe en la auditoría que alguien hizo un paso sobre una factura. Es
// EXACTAMENTE de donde salen las firmas del papel.
func sembrarEvento(t *testing.T, r *pgRepository, docID, accion, usuarioID string) {
	t.Helper()
	if _, err := r.pool.Exec(context.Background(), `
		INSERT INTO auditoria_evento (empresa_id, entidad, entidad_id, accion, usuario_id)
		VALUES ($1::uuid, 'documento_cxp', $2::uuid, $3, $4::uuid)`,
		empresaLote, docID, accion, usuarioID); err != nil {
		t.Fatalf("sembrar evento de auditoría: %v", err)
	}
}

// firmaPorRol busca una de las cuatro líneas.
func firmaPorRol(t *testing.T, firmas []Firma, rol string) Firma {
	t.Helper()
	for _, f := range firmas {
		if f.Rol == rol {
			return f
		}
	}
	t.Fatalf("el papel no trae la línea %q; las cuatro van siempre, aunque estén vacías", rol)
	return Firma{}
}

func TestLasFirmasSalenDeQuienHizoCadaPaso(t *testing.T) {
	repo := baseDeLotes(t)
	ctx := context.Background()

	// Un corte con una factura, creado por el usuario de prueba.
	loteID, docID := sembrarCorteConFactura(t, repo)

	// Dos personas revisan (es lo normal con varias facturas) y una aprueba.
	otro := sembrarUsuario(t, repo, "Jossabeth Castro", "jossabeth@prueba.local")
	sembrarEvento(t, repo, docID, "REVISAR_DOCUMENTO", usuarioLote)
	sembrarEvento(t, repo, docID, "REVISAR_DOCUMENTO", otro)
	sembrarEvento(t, repo, docID, "APROBAR_DOCUMENTO", otro)

	firmas, fechaPago, err := repo.FirmasDelLote(ctx, empresaLote, loteID)
	if err != nil {
		t.Fatalf("firmas: %v", err)
	}
	if len(firmas) != 4 {
		t.Fatalf("firmas = %d, se esperaban las cuatro líneas siempre", len(firmas))
	}

	// Solicitado: quien armó el corte, de la ficha del corte.
	if sol := firmaPorRol(t, firmas, RolSolicitado); sol.Nombre == "" {
		t.Error("«Solicitado por» salió vacío: el corte sí tiene quién lo creó")
	}

	// Revisado: LOS DOS. Si saliera uno solo, el papel diría que el otro no participó.
	rev := firmaPorRol(t, firmas, RolRevisado)
	if !strings.Contains(rev.Nombre, "Jossabeth Castro") {
		t.Errorf("«Revisado por» = %q: falta quien también revisó", rev.Nombre)
	}
	if !strings.Contains(rev.Nombre, " / ") {
		t.Errorf("«Revisado por» = %q: se esperaban los dos revisores separados por « / »", rev.Nombre)
	}

	apr := firmaPorRol(t, firmas, RolAprobado)
	if apr.Nombre != "Jossabeth Castro" {
		t.Errorf("«Aprobado por» = %q, se esperaba Jossabeth Castro", apr.Nombre)
	}

	// PAGADO VA VACÍO: el corte no se cerró. Es el caso que importa —un papel que pusiera un
	// nombre acá estaría afirmando un pago que no ocurrió—.
	pag := firmaPorRol(t, firmas, RolPagado)
	if pag.Nombre != "" {
		t.Errorf("«Pagado por» = %q sobre un corte sin cerrar: el papel está firmando un pago que no pasó", pag.Nombre)
	}
	if fechaPago != "" {
		t.Errorf("fecha de pago = %q sobre un corte sin cerrar", fechaPago)
	}
}

// La auditoría de OTRO corte no puede aparecer en este papel.
func TestLasFirmasNoTraenLaAuditoriaDeOtroCorte(t *testing.T) {
	repo := baseDeLotes(t)
	ctx := context.Background()

	loteA, _ := sembrarCorteConFactura(t, repo)
	_, docB := sembrarCorteConFactura(t, repo)

	ajeno := sembrarUsuario(t, repo, "Persona Del Otro Corte", "ajeno@prueba.local")
	sembrarEvento(t, repo, docB, "APROBAR_DOCUMENTO", ajeno)

	firmas, _, err := repo.FirmasDelLote(ctx, empresaLote, loteA)
	if err != nil {
		t.Fatalf("firmas: %v", err)
	}
	if apr := firmaPorRol(t, firmas, RolAprobado); apr.Nombre != "" {
		t.Errorf("«Aprobado por» = %q: se coló la auditoría de otro corte", apr.Nombre)
	}
}
