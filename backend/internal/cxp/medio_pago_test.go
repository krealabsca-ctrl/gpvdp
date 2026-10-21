package cxp

// Cerrar una factura que ya se pagó por fuera del banco (mig 0102).
//
// EL CASO REAL (Director Financiero, 16-oct-2026): facturas de contado que ya se habían pagado por
// caja chica y que el sistema solo dejaba ANULAR. Anular dice «esta factura no existió» cuando el
// dinero salió de verdad: el gasto desaparece de los libros y el proveedor queda sin su cuenta
// saldada.
//
// La causa era una asimetría de transiciones: `Anular` llega hasta PROGRAMADO y `Liquidar` se
// quedaba en REVISADO, así que en «Por pagar» la única salida ofrecida era la deshonesta.

import (
	"context"
	"strings"
	"testing"
)

func servicioMedio(repo *fakeRepo) *Service { return servicioHuella(repo) }

// El cierre honesto tiene que llegar TAN LEJOS como anular. Si no, la gente sigue anulando — que
// es el problema que esto viene a resolver.
func TestSeCierraDesdeLosMismosEstadosQueSePuedeAnular(t *testing.T) {
	t.Parallel()
	anulables := map[string]bool{}
	for _, e := range []string{EstRecibido, EstRevisado, EstValidadoDepto, EstAprobado, EstProgramado} {
		anulables[e] = true
	}
	for _, e := range estadosQueSePuedenCerrarPorOtroMedio {
		if !anulables[e] {
			t.Errorf("%s se puede cerrar por otro medio pero no anular: la lista se desalineó", e)
		}
		delete(anulables, e)
	}
	for e := range anulables {
		t.Errorf("desde %s se puede ANULAR pero no cerrar por otro medio; ahí vuelve el problema", e)
	}
}

func TestCerrarPorOtroMedioDejaLaFacturaLiquidadaYGuardaElMedio(t *testing.T) {
	t.Parallel()
	repo := &fakeRepo{filasCambio: 1}
	svc := servicioMedio(repo)

	_, salioDeUnLote, err := svc.CerrarPorOtroMedio(context.Background(), "e1", "doc-1", MedioPagoInput{
		Medio:  MedioCajaChica,
		Motivo: "vale 214 de caja chica del 3 de setiembre, fondo de Cartago",
	}, "u1")
	if err != nil {
		t.Fatalf("CerrarPorOtroMedio: %v", err)
	}
	if salioDeUnLote {
		t.Error("no estaba en un lote y dijo que salió de uno")
	}

	// LIQUIDADA y no PAGADO: en PAGADO entraría en la partida conciliatoria del tablero y nunca
	// conciliaría, porque no hay débito con huella que la respalde.
	if repo.capMultiA != EstLiquidada {
		t.Errorf("estado destino = %q, se esperaba LIQUIDADA", repo.capA)
	}
	if repo.medioGuardado == nil {
		t.Fatal("no guardó el medio: una LIQUIDADA sin medio es indistinguible de un viático archivado")
	}
	if repo.medioGuardado.Medio != MedioCajaChica {
		t.Errorf("medio = %q", repo.medioGuardado.Medio)
	}
	if repo.medioGuardadoPor != "u1" {
		t.Errorf("no registró quién lo declaró: %q", repo.medioGuardadoPor)
	}
}

// El motivo es la única barrera contra el doble pago: el vale de caja chica no lleva la clave del
// comprobante, así que nada más puede cruzarlos.
func TestSinMotivoNoSeCierra(t *testing.T) {
	t.Parallel()
	repo := &fakeRepo{filasCambio: 1}
	for _, motivo := range []string{"", "ok", "ya se pagó"} {
		_, _, err := servicioMedio(repo).CerrarPorOtroMedio(context.Background(), "e1", "doc-1",
			MedioPagoInput{Medio: MedioEfectivo, Motivo: motivo}, "u1")
		if err != ErrMotivoMedioPagoRequerido {
			t.Errorf("motivo %q: err = %v, se esperaba ErrMotivoMedioPagoRequerido", motivo, err)
		}
	}
	if repo.capMultiA != "" {
		t.Error("cambió el estado sin motivo")
	}
	if repo.medioGuardado != nil {
		t.Error("guardó el medio sin motivo")
	}
}

func TestElMedioTieneQueSerUnoDeLosCuatro(t *testing.T) {
	t.Parallel()
	repo := &fakeRepo{filasCambio: 1}
	motivo := "se pagó con la tarjeta corporativa el 2 de octubre"

	for _, m := range []string{MedioCajaChica, MedioViaticos, MedioTarjeta, MedioEfectivo} {
		in := MedioPagoInput{Medio: m, Motivo: motivo}
		if err := in.Normalizar(); err != nil {
			t.Errorf("%s debería ser válido: %v", m, err)
		}
	}
	// Minúsculas y espacios no son otro medio.
	in := MedioPagoInput{Medio: "  caja_chica ", Motivo: motivo}
	if err := in.Normalizar(); err != nil || in.Medio != MedioCajaChica {
		t.Errorf("no normalizó: %q, %v", in.Medio, err)
	}
	// Cualquier otra cosa, no.
	for _, m := range []string{"", "CHEQUE", "SINPE", "CAJA CHICA"} {
		mal := MedioPagoInput{Medio: m, Motivo: motivo}
		if err := mal.Normalizar(); err != ErrMedioPagoInvalido {
			t.Errorf("medio %q: err = %v, se esperaba ErrMedioPagoInvalido", m, err)
		}
	}
	_ = repo
}

// El vale es del fondo fijo: ligarlo a una tarjeta diría que la plata salió de la caja.
func TestElValeSoloSeLigaACajaChica(t *testing.T) {
	t.Parallel()
	in := MedioPagoInput{
		Medio: MedioTarjeta, ValeID: "vale-1",
		Motivo: "se pagó con la tarjeta corporativa el 2 de octubre",
	}
	if err := in.Normalizar(); err != ErrValeSoloCajaChica {
		t.Errorf("err = %v, se esperaba ErrValeSoloCajaChica", err)
	}
}

// El vale es OPCIONAL (decisión del Director Financiero): exigirlo dejaría la factura trancada
// cuando nadie lo cargó, que es el mismo problema que esto resuelve. Pero si se declara uno, tiene
// que existir: un id muerto diría que hay respaldo donde no lo hay.
func TestElValeEsOpcionalPeroSiSeDeclaraTieneQueExistir(t *testing.T) {
	t.Parallel()
	motivo := "vale 214 de caja chica del 3 de setiembre, fondo de Cartago"

	t.Run("sin vale se cierra igual", func(t *testing.T) {
		repo := &fakeRepo{filasCambio: 1}
		if _, _, err := servicioMedio(repo).CerrarPorOtroMedio(context.Background(), "e1", "doc-1",
			MedioPagoInput{Medio: MedioCajaChica, Motivo: motivo}, "u1"); err != nil {
			t.Fatalf("sin vale tenía que poder cerrarse: %v", err)
		}
		if repo.capMultiA != EstLiquidada {
			t.Error("no la liquidó")
		}
	})

	t.Run("con un vale que no existe, no se cierra", func(t *testing.T) {
		repo := &fakeRepo{filasCambio: 1, valesQueExisten: map[string]bool{}}
		_, _, err := servicioMedio(repo).CerrarPorOtroMedio(context.Background(), "e1", "doc-1",
			MedioPagoInput{Medio: MedioCajaChica, Motivo: motivo, ValeID: "vale-fantasma"}, "u1")
		if err != ErrValeNoEncontrado {
			t.Fatalf("err = %v, se esperaba ErrValeNoEncontrado", err)
		}
		// Y sobre todo: no tocó la factura.
		if repo.capMultiA != "" {
			t.Error("liquidó la factura con un vale inexistente")
		}
	})

	t.Run("con un vale real se liga", func(t *testing.T) {
		repo := &fakeRepo{filasCambio: 1, valesQueExisten: map[string]bool{"vale-1": true}}
		if _, _, err := servicioMedio(repo).CerrarPorOtroMedio(context.Background(), "e1", "doc-1",
			MedioPagoInput{Medio: MedioCajaChica, Motivo: motivo, ValeID: "vale-1"}, "u1"); err != nil {
			t.Fatalf("CerrarPorOtroMedio: %v", err)
		}
		if repo.medioGuardado == nil || repo.medioGuardado.ValeID != "vale-1" {
			t.Error("no guardó el vale")
		}
	})
}

// Cerrar una factura que estaba en un corte le quita una línea a ese corte. Quien lo hizo tiene
// que enterarse en el momento, no cuando baje la macro y le falte una.
func TestSiSaleDeUnLoteSeAvisa(t *testing.T) {
	t.Parallel()
	repo := &fakeRepo{filasCambio: 1, medioEstabaEnLote: true}
	_, salioDeUnLote, err := servicioMedio(repo).CerrarPorOtroMedio(context.Background(), "e1", "doc-1",
		MedioPagoInput{Medio: MedioCajaChica, Motivo: "vale 214 de caja chica del 3 de setiembre"}, "u1")
	if err != nil {
		t.Fatalf("CerrarPorOtroMedio: %v", err)
	}
	if !salioDeUnLote {
		t.Error("salió de un lote y no lo dijo: el corte de otra persona perdió una línea en silencio")
	}
}

// Una factura ya CONCILIADA no se puede «cerrar por otro medio»: el banco ya la pagó, y decir que
// salió por caja chica sería tapar un doble pago en vez de reportarlo.
func TestUnaFacturaFueraDeEstadoNoSeCierra(t *testing.T) {
	t.Parallel()
	repo := &fakeRepo{multiRechaza: true, doc: Documento{ID: "doc-1", Estado: EstConciliado}}
	_, _, err := servicioMedio(repo).CerrarPorOtroMedio(context.Background(), "e1", "doc-1",
		MedioPagoInput{Medio: MedioCajaChica, Motivo: "vale 214 de caja chica del 3 de setiembre"}, "u1")
	if err == nil {
		t.Fatal("cerró una factura ya conciliada")
	}
	if repo.medioGuardado != nil {
		t.Error("escribió el medio sobre una factura que no cambió de estado")
	}
}

func TestLaEtiquetaDelMedioSeLee(t *testing.T) {
	t.Parallel()
	for medio, quiere := range map[string]string{
		MedioCajaChica: "caja chica", MedioViaticos: "viáticos",
		MedioTarjeta: "tarjeta", MedioEfectivo: "efectivo",
	} {
		if got := EtiquetaMedioPago(medio); got != quiere {
			t.Errorf("%s → %q, se esperaba %q", medio, got, quiere)
		}
	}
	// Un medio desconocido se muestra tal cual en vez de desaparecer.
	if got := EtiquetaMedioPago("X"); got != "X" {
		t.Errorf("medio desconocido = %q", got)
	}
}

// El motivo tiene que quedar en la línea de tiempo del documento, con el medio adelante: es donde
// alguien va a buscar «¿por qué esta factura está liquidada?».
func TestLaNotaDeLaLineaDeTiempoNombraElMedio(t *testing.T) {
	t.Parallel()
	motivo := "vale 214 del fondo de Cartago, 3 de setiembre"

	nota := NotaDeCierre(MedioCajaChica, motivo, false)
	if !strings.Contains(nota, "caja chica") {
		t.Errorf("la nota no nombra el medio: %q", nota)
	}
	if !strings.Contains(nota, motivo) {
		t.Errorf("la nota pierde el motivo, que es la mitad del valor: %q", nota)
	}
	if strings.Contains(nota, "corte") {
		t.Errorf("no salió de ningún corte y lo dice igual: %q", nota)
	}

	// Salir de un corte es un efecto sobre OTRO objeto y tiene que quedar escrito.
	conLote := NotaDeCierre(MedioCajaChica, motivo, true)
	if !strings.Contains(conLote, "corte") {
		t.Errorf("salió de un lote y la nota no lo dice: %q", conLote)
	}
}
