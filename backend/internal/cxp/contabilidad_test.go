package cxp

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"

	"github.com/gpvdp/erp/internal/shared"
)

// permisosFake responde el set de permisos que se le dé, para probar el candado del permiso propio
// sin levantar RBAC.
type permisosFake struct{ tiene map[string]bool }

func (p permisosFake) Tiene(_ context.Context, _, _, permiso string) (bool, error) {
	return p.tiene[permiso], nil
}

func servicioConta(t *testing.T, repo *fakeRepo, permisos map[string]bool) *Service {
	t.Helper()
	s := NewService(repo, shared.NewAudit(nil, zap.NewNop()), zap.NewNop())
	if permisos != nil {
		s.SetPermisos(permisosFake{tiene: permisos})
	}
	return s
}

// El candado central: una factura «de Contabilidad» se aprueba SIN validación de área, pero solo
// por quien tiene el permiso propio. Sin él es 403, no un pase libre.
func TestAprobarFacturaDeContabilidad(t *testing.T) {
	t.Parallel()

	casos := []struct {
		nombre      string
		estado      string
		origen      string
		permiso     bool
		errEsperado error
	}{
		{"marcada por el proveedor, con permiso, desde REVISADO", EstRevisado, ContaOrigenProveedor, true, nil},
		{"marcada a mano, con permiso, desde RECIBIDO", EstRecibido, ContaOrigenFactura, true, nil},
		{"marcada por el concepto, con permiso, ya validada por el área", EstValidadoDepto, ContaOrigenConcepto, true, nil},
		{"marcada pero SIN el permiso propio", EstRevisado, ContaOrigenProveedor, false, ErrNoAprobadorContabilidad},
		{"NO marcada: sigue exigiendo validación de área", EstRevisado, "", true, ErrTransicionInvalida},
		{"marcada pero ya pagada", EstPagado, ContaOrigenProveedor, true, ErrTransicionInvalida},
	}

	for _, c := range casos {
		c := c
		t.Run(c.nombre, func(t *testing.T) {
			t.Parallel()
			repo := &fakeRepo{
				doc: Documento{
					ID: "d1", Tipo: "CXP", Estado: c.estado,
					TotalCRC: "50000", NetoCRC: "50000",
					ContabilidadOrigen: c.origen, EsContabilidad: c.origen != "",
				},
				filasCambio: 1,
			}
			s := servicioConta(t, repo, map[string]bool{permisoAprobarContabilidad: c.permiso})

			_, err := s.Aprobar(context.Background(), "e1", "d1", "u1", "SUPERVISOR_FINANCIERO")
			if !errors.Is(err, c.errEsperado) {
				t.Fatalf("Aprobar() error = %v, se esperaba %v", err, c.errEsperado)
			}
		})
	}
}

// La segregación de funciones NO se relaja por estar marcada: quien validó por el área sigue sin
// poder aprobar la misma factura.
func TestFacturaDeContabilidadRespetaSegregacion(t *testing.T) {
	t.Parallel()
	repo := &fakeRepo{
		doc: Documento{
			ID: "d1", Tipo: "CXP", Estado: EstValidadoDepto, TotalCRC: "50000", NetoCRC: "50000",
			ContabilidadOrigen: ContaOrigenProveedor, EsContabilidad: true,
			ValidadoDeptoPor: "u1",
		},
		filasCambio: 1,
	}
	s := servicioConta(t, repo, map[string]bool{permisoAprobarContabilidad: true})

	if _, err := s.Aprobar(context.Background(), "e1", "d1", "u1", "DIRECTOR_FINANCIERO"); !errors.Is(err, ErrValidadorNoAprueba) {
		t.Fatalf("error = %v, se esperaba ErrValidadorNoAprueba", err)
	}
}

// Segregación propia de la excepción: quien MARCÓ a mano la factura no puede además firmarla.
// Sin esto, el SUPERVISOR_FINANCIERO —que tiene los dos permisos por defecto— cerraría el ciclo
// completo solo: marcar, aprobar y dejarla lista para pago.
func TestQuienMarcaNoAprueba(t *testing.T) {
	t.Parallel()

	casos := []struct {
		nombre      string
		origen      string
		marcadoPor  string
		actor       string
		errEsperado error
	}{
		{"el que marcó a mano no aprueba", ContaOrigenFactura, "u1", "u1", ErrMarcadorNoAprueba},
		{"otro usuario sí aprueba lo que marcó u1", ContaOrigenFactura, "u1", "u2", nil},
		// La marca heredada no es una decisión sobre ESTA factura: no bloquea a nadie.
		{"marca heredada del proveedor: no bloquea", ContaOrigenProveedor, "", "u1", nil},
		{"marca heredada del concepto: no bloquea", ContaOrigenConcepto, "", "u1", nil},
	}

	for _, c := range casos {
		c := c
		t.Run(c.nombre, func(t *testing.T) {
			t.Parallel()
			repo := &fakeRepo{
				doc: Documento{
					ID: "d1", Tipo: "CXP", Estado: EstRevisado, TotalCRC: "50000", NetoCRC: "50000",
					ContabilidadOrigen: c.origen, EsContabilidad: true,
					ContabilidadMarcadoPor: c.marcadoPor,
				},
				filasCambio: 1,
			}
			s := servicioConta(t, repo, map[string]bool{permisoAprobarContabilidad: true})

			_, err := s.Aprobar(context.Background(), "e1", "d1", c.actor, "SUPERVISOR_FINANCIERO")
			if !errors.Is(err, c.errEsperado) {
				t.Fatalf("Aprobar() error = %v, se esperaba %v", err, c.errEsperado)
			}
		})
	}
}

// El permiso propio ABRE la puerta a quien no tiene cxp.aprobar; no se la cierra a quien sí lo
// tiene. Marcar un rubro no puede dejar a los aprobadores normales sin poder aprobar.
func TestElAprobadorGeneralTambienFirmaLasDeContabilidad(t *testing.T) {
	t.Parallel()
	repo := &fakeRepo{
		doc: Documento{
			ID: "d1", Tipo: "CXP", Estado: EstRevisado, TotalCRC: "50000", NetoCRC: "50000",
			ContabilidadOrigen: ContaOrigenProveedor, EsContabilidad: true,
		},
		filasCambio: 1,
	}
	// Solo el permiso GENERAL, sin el propio.
	s := servicioConta(t, repo, map[string]bool{
		permisoAprobarContabilidad: false,
		permisoAprobar:             true,
	})

	if _, err := s.Aprobar(context.Background(), "e1", "d1", "u1", "GERENCIA_GENERAL"); err != nil {
		t.Fatalf("con cxp.aprobar debería poder aprobar; error = %v", err)
	}
}

// Y sin NINGUNO de los dos, no.
func TestSinNingunPermisoNoApruebaLasDeContabilidad(t *testing.T) {
	t.Parallel()
	repo := &fakeRepo{
		doc: Documento{
			ID: "d1", Tipo: "CXP", Estado: EstRevisado, TotalCRC: "50000", NetoCRC: "50000",
			ContabilidadOrigen: ContaOrigenProveedor, EsContabilidad: true,
		},
		filasCambio: 1,
	}
	s := servicioConta(t, repo, map[string]bool{permisoAprobarContabilidad: false, permisoAprobar: false})

	if _, err := s.Aprobar(context.Background(), "e1", "d1", "u1", "AUXILIAR_FINANCIERO"); !errors.Is(err, ErrNoAprobadorContabilidad) {
		t.Fatalf("error = %v, se esperaba ErrNoAprobadorContabilidad", err)
	}
}

// Al aprobar, la marca HEREDADA se sella en la factura. Si no se sellara, desmarcar el proveedor
// mañana borraría del documento la razón por la que se aprobó sin validación de área.
func TestAprobarSellaLaMarcaHeredada(t *testing.T) {
	t.Parallel()

	casos := []struct {
		nombre        string
		origen        string
		neto          string
		sellaEsperado bool
	}{
		{"heredada del proveedor y queda aprobada: sella", ContaOrigenProveedor, "50000", true},
		{"heredada del concepto y queda aprobada: sella", ContaOrigenConcepto, "50000", true},
		// Marcada a mano: ya está en su propia columna, no hay nada que congelar.
		{"marcada a mano: no sella", ContaOrigenFactura, "50000", false},
		// Con firmas a medias (> ₡1M pide 2) todavía no pasó nada que congelar.
		{"heredada pero faltan firmas: no sella", ContaOrigenProveedor, "3000000", false},
	}

	for _, c := range casos {
		c := c
		t.Run(c.nombre, func(t *testing.T) {
			t.Parallel()
			repo := &fakeRepo{
				doc: Documento{
					ID: "d1", Tipo: "CXP", Estado: EstRevisado, TotalCRC: c.neto, NetoCRC: c.neto,
					ContabilidadOrigen: c.origen, EsContabilidad: true,
				},
				filasCambio: 1,
				// UNA firma registrada: alcanza para ≤ ₡1M y no para el caso de ₡3M, que es lo
				// que distingue «quedó aprobada» de «faltan firmas».
				rolesAprobaciones: []string{"SUPERVISOR_FINANCIERO"},
			}
			s := servicioConta(t, repo, map[string]bool{permisoAprobarContabilidad: true})

			if _, err := s.Aprobar(context.Background(), "e1", "d1", "u1", "SUPERVISOR_FINANCIERO"); err != nil {
				t.Fatalf("Aprobar() error = %v", err)
			}
			if repo.contaSellado != c.sellaEsperado {
				t.Errorf("sellado = %v, se esperaba %v", repo.contaSellado, c.sellaEsperado)
			}
			// Y el sello dice de dónde venía la marca: sin eso el documento no explica nada.
			if c.sellaEsperado && repo.contaSelloMotivo == "" {
				t.Error("el sello quedó sin motivo: el documento no explicaría por qué se saltó el área")
			}
		})
	}
}

// La vía propia (`aprobar-contabilidad`) NO sirve para aprobar cualquier factura: si no está
// marcada, se rechaza. Sin este candado, cxp.aprobar_contabilidad sería un cxp.aprobar que además
// se salta la validación de área.
func TestViaDeContabilidadRechazaFacturaNoMarcada(t *testing.T) {
	t.Parallel()
	repo := &fakeRepo{
		doc:         Documento{ID: "d1", Tipo: "CXP", Estado: EstRevisado, TotalCRC: "50000", NetoCRC: "50000"},
		filasCambio: 1,
	}
	s := servicioConta(t, repo, map[string]bool{permisoAprobarContabilidad: true})

	if _, err := s.AprobarContabilidad(context.Background(), "e1", "d1", "u1", "SUPERVISOR_FINANCIERO"); !errors.Is(err, ErrNoEsDeContabilidad) {
		t.Fatalf("error = %v, se esperaba ErrNoEsDeContabilidad", err)
	}
}

// Marcar UNA factura a mano exige motivo; desmarcar y volver a heredar no.
func TestMarcarDocumentoContabilidadMotivo(t *testing.T) {
	t.Parallel()
	si, no := true, false

	casos := []struct {
		nombre      string
		valor       *bool
		motivo      string
		errEsperado error
		guardaMotiv string
	}{
		{"marcar sin motivo", &si, "", ErrMotivoContabilidadRequerido, ""},
		{"marcar sin motivo (solo espacios)", &si, "   ", ErrMotivoContabilidadRequerido, ""},
		{"marcar con motivo", &si, "honorarios contables del mes", nil, "honorarios contables del mes"},
		{"forzar que la valide el área", &no, "", nil, ""},
		{"volver a heredar", nil, "", nil, ""},
		{"desmarcar descarta el motivo viejo", &no, "algo", nil, ""},
	}

	for _, c := range casos {
		c := c
		t.Run(c.nombre, func(t *testing.T) {
			t.Parallel()
			repo := &fakeRepo{doc: Documento{ID: "d1", Tipo: "CXP", Estado: EstRevisado}}
			s := servicioConta(t, repo, nil)

			_, err := s.MarcarDocumentoContabilidad(context.Background(), "e1", "d1", c.valor, c.motivo, "u1")
			if !errors.Is(err, c.errEsperado) {
				t.Fatalf("error = %v, se esperaba %v", err, c.errEsperado)
			}
			if c.errEsperado != nil {
				return
			}
			if repo.contaMotivoDoc != c.guardaMotiv {
				t.Errorf("motivo guardado = %q, se esperaba %q", repo.contaMotivoDoc, c.guardaMotiv)
			}
			// El valor viaja tal cual (los tres estados se distinguen).
			switch {
			case c.valor == nil && repo.contaMarcaDoc != nil:
				t.Errorf("se esperaba NULL y llegó %v", *repo.contaMarcaDoc)
			case c.valor != nil && repo.contaMarcaDoc == nil:
				t.Errorf("se esperaba %v y llegó NULL", *c.valor)
			case c.valor != nil && *repo.contaMarcaDoc != *c.valor:
				t.Errorf("valor = %v, se esperaba %v", *repo.contaMarcaDoc, *c.valor)
			}
		})
	}
}

// Una factura ya aprobada o pagada no cambia de marca: reescribirla borraría el rastro de por qué
// se aprobó sin pasar por el área.
func TestMarcarDocumentoContabilidadSoloAntesDeAprobar(t *testing.T) {
	t.Parallel()
	si := true
	repo := &fakeRepo{
		doc:            Documento{ID: "d1", Tipo: "CXP", Estado: EstPagado},
		contaFilasCero: true, // el UPDATE no alcanzó ninguna fila (estado fuera del tramo)
	}
	s := servicioConta(t, repo, nil)

	if _, err := s.MarcarDocumentoContabilidad(context.Background(), "e1", "d1", &si, "porque sí", "u1"); !errors.Is(err, ErrContabilidadNoModificable) {
		t.Fatalf("error = %v, se esperaba ErrContabilidadNoModificable", err)
	}
}

// La auditoría tiene que decir QUÉ pasó: un evento que solo diga «MARCAR» obliga a abrir la fila.
func TestDescripcionMarca(t *testing.T) {
	t.Parallel()
	si, no := true, false

	if got := descripcionMarca(&si, "timbres"); got != "de Contabilidad (sin validación de área) — timbres" {
		t.Errorf("marcar: %q", got)
	}
	if got := descripcionMarca(&no, ""); got == "" {
		t.Error("desmarcar no dejó descripción")
	}
	if got := descripcionMarca(nil, ""); got != "vuelve a heredar la marca del proveedor/rubro" {
		t.Errorf("heredar: %q", got)
	}
}

// accionMarca separa marcar de desmarcar para poder filtrarlas aparte en el histórico.
func TestAccionMarcaDistingueMarcarDeDesmarcar(t *testing.T) {
	t.Parallel()
	if got := accionMarca("MARCAR_PROVEEDOR_CONTABILIDAD", true); got != "MARCAR_PROVEEDOR_CONTABILIDAD" {
		t.Errorf("marcar = %q", got)
	}
	if got := accionMarca("MARCAR_PROVEEDOR_CONTABILIDAD", false); got != "DESMARCAR_PROVEEDOR_CONTABILIDAD" {
		t.Errorf("desmarcar = %q", got)
	}
}

// La etiqueta del origen es lo que la pantalla y la auditoría muestran para explicar la marca.
func TestEtiquetaOrigenContabilidad(t *testing.T) {
	t.Parallel()
	for _, origen := range []string{ContaOrigenFactura, ContaOrigenProveedor, ContaOrigenClasificacion, ContaOrigenConcepto} {
		if EtiquetaOrigenContabilidad(origen) == "" {
			t.Errorf("el origen %q no tiene explicación", origen)
		}
	}
	if EtiquetaOrigenContabilidad("") != "" {
		t.Error("sin origen no debería haber explicación")
	}
}
