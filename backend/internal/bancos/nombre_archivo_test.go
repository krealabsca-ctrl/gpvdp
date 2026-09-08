package bancos

import (
	"strings"
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

// El nombre que el usuario usa para archivar (2026-09-03): empresa, clasificaciones abreviadas, mes
// de los datos y fecha de descarga. Los tres primeros casos son los que él escribe a diario.
func TestNombreArchivoReporte(t *testing.T) {
	// 3 de setiembre de 2026 → 03092026.
	en := time.Date(2026, 9, 3, 20, 39, 0, 0, time.UTC)
	casos := []struct{ empresa, clasifs, mes, detalle, quiere string }{
		{"Valle de Paz", "Asoc + Dep", "Agosto", "", "VDP Asoc + Dep Agosto 03092026.xlsx"},
		{"Valle de Paz", "Dep", "Agosto", "", "VDP Dep Agosto 03092026.xlsx"},
		{"Valle de Paz", "Serv + Dep", "Agosto", "", "VDP Serv + Dep Agosto 03092026.xlsx"},
		{"Valle de Paz", "Completo", "Agosto", "", "VDP Completo Agosto 03092026.xlsx"},
		{"Coopeprofa", "Completo", "Julio", "", "CPF Completo Julio 03092026.xlsx"},
		{"Memorial Pets", "Dep", "Setiembre", "", "MPTS Dep Setiembre 03092026.xlsx"},
		// El cuadre sí lleva detalle: es otro reporte y no puede pisar al detalle en la carpeta.
		{"Coopeprofa", "", "", "cuadre", "CPF cuadre 03092026.xlsx"},
		// Partes en blanco no dejan espacios colgando.
		{"Valle de Paz", "", "", "   ", "VDP 03092026.xlsx"},
		// La barra de «Asociaciones / Cooperativas» no puede llegar al nombre: partiría el
		// Content-Disposition y el archivo bajaría con otro nombre.
		{"Valle de Paz", "Asoc / Coop", "Agosto", "", "VDP Asoc Coop Agosto 03092026.xlsx"},
	}
	for _, c := range casos {
		got := NombreArchivoReporte(c.empresa, c.clasifs, c.mes, en, c.detalle)
		if got != c.quiere {
			t.Errorf("NombreArchivoReporte(%q, %q, %q, %q) = %q, quiere %q",
				c.empresa, c.clasifs, c.mes, c.detalle, got, c.quiere)
		}
	}
}

// La palabra «corrido» ya NO va en el nombre (el usuario la pidió fuera). Bajar la vista agrupada y
// la corrida el mismo día con el mismo filtro da el mismo nombre a propósito: para él son el mismo
// archivo con otra presentación.
func TestElNombreYaNoDiceCorrido(t *testing.T) {
	en := time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC)
	movs := []MovimientoExport{{Clasificacion: "Deposito de Clientes", ClasificacionID: "algo"}}
	f := FiltrosMovimientos{Periodo: "2026-08", ClasificacionID: "algo"}
	for _, nombre := range []string{
		nombreArchivoMovimientos("Valle de Paz", f, movs),
		NombreArchivoReporte("Valle de Paz", "Dep", "Agosto", en, ""),
	} {
		if strings.Contains(strings.ToLower(nombre), "corrid") {
			t.Errorf("el nombre volvió a decir «corrido»: %q", nombre)
		}
	}
}

func TestAbreviarClasificacion(t *testing.T) {
	casos := []struct{ nombre, quiere string }{
		// Las tres que el usuario escribe a mano.
		{"Asociaciones", "Asoc"},
		{"Asociaciones / Cooperativas", "Asoc"},
		{"Deposito de Clientes", "Dep"},
		{"Depósito de Clientes", "Dep"}, // con tilde cae en la misma
		{"Servicios de Emergencias", "Serv"},
		// Regla de respaldo: primeras 4 letras de la primera palabra.
		{"Datafonos", "Data"},
		{"Planilla", "Plan"},
		{"CCSS", "Ccss"},
		{"Cargo BN", "Carg"},
		{"IVA", "Iva"},
		{"", ""},
		{"   ", ""},
	}
	for _, c := range casos {
		if got := AbreviarClasificacion(c.nombre); got != c.quiere {
			t.Errorf("AbreviarClasificacion(%q) = %q, quiere %q", c.nombre, got, c.quiere)
		}
	}
}

func TestEtiquetaClasificaciones(t *testing.T) {
	casos := []struct {
		nombre string
		in     []string
		quiere string
	}{
		{"sin filtro es Completo", nil, "Completo"},
		{"una", []string{"Deposito de Clientes"}, "Dep"},
		{"dos, en el orden pedido", []string{"Asociaciones", "Deposito de Clientes"}, "Asoc + Dep"},
		{"tres entran enteras", []string{"Servicios de Emergencias", "Deposito de Clientes", "Asociaciones"}, "Serv + Dep + Asoc"},
		// Más de tres se resume: Windows corta los nombres en 255 caracteres.
		{"cuatro se resumen", []string{"Asociaciones", "Deposito de Clientes", "Servicios de Emergencias", "Datafonos"}, "Asoc + Dep + Serv +1 más"},
		{"seis se resumen", []string{"Asociaciones", "Deposito de Clientes", "Servicios de Emergencias", "Datafonos", "Planilla", "CCSS"}, "Asoc + Dep + Serv +3 más"},
		// Dos clasificaciones distintas con la MISMA abreviatura no la repiten.
		{"sin repetir la abreviatura", []string{"Servicios de Emergencias", "Servicios Funerarios"}, "Serv"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got := EtiquetaClasificaciones(c.in); got != c.quiere {
				t.Errorf("EtiquetaClasificaciones(%v) = %q, quiere %q", c.in, got, c.quiere)
			}
		})
	}
}

// El mes es el de los DATOS, no el de hoy: es lo que distingue bajar agosto en setiembre.
func TestMesDelReporte(t *testing.T) {
	hoy := time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC)
	casos := []struct {
		nombre string
		f      FiltrosMovimientos
		quiere string
	}{
		{"un período", FiltrosMovimientos{Periodo: "2026-08"}, "Agosto"},
		{"la ortografía tica", FiltrosMovimientos{Periodo: "2026-09"}, "Setiembre"},
		{"varios períodos", FiltrosMovimientos{Periodos: []string{"2026-06", "2026-07", "2026-08"}}, "Junio a Agosto"},
		{"desordenados se ordenan", FiltrosMovimientos{Periodos: []string{"2026-08", "2026-06"}}, "Junio a Agosto"},
		{"rango de fechas de un mes", FiltrosMovimientos{Desde: "2026-08-01", Hasta: "2026-08-31"}, "Agosto"},
		{"rango de fechas de dos meses", FiltrosMovimientos{Desde: "2026-07-15", Hasta: "2026-08-10"}, "Julio a Agosto"},
		// Otro año: se agrega, porque la fecha del final lleva el año de HOY y sin esto dos
		// descargas del mismo mes de años distintos se llamarían igual.
		{"otro año lo dice", FiltrosMovimientos{Periodo: "2025-08"}, "Agosto 2025"},
		{"sin período", FiltrosMovimientos{}, "Historico"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got := MesDelReporte(c.f, hoy); got != c.quiere {
				t.Errorf("MesDelReporte = %q, quiere %q", got, c.quiere)
			}
		})
	}
}

// Las abreviaturas salen en el ORDEN EN QUE SE ELIGIERON las clasificaciones (decisión del usuario):
// es el orden en que él escribe el nombre a mano, «Serv + Dep» y no «Dep + Serv».
//
// El caso de abajo lo prueba con el orden invertido respecto al alfabético: si alguien volviera a
// ordenar alfabéticamente, este test se pone rojo.
func TestNombreRespetaElOrdenEnQueSeEligieron(t *testing.T) {
	hoy := AhoraCR().Format("02012006")
	// Elegidas: Servicios primero, Depósito después. Alfabéticamente sería «Dep + Serv».
	f := FiltrosMovimientos{Periodo: "2026-08", ClasificacionIDs: []string{"id-serv", "id-dep"}}
	// Los movimientos llegan en otro orden todavía: manda el filtro, no la aparición.
	movs := []MovimientoExport{
		{Clasificacion: "Deposito de Clientes", ClasificacionID: "id-dep"},
		{Clasificacion: "Servicios de Emergencias", ClasificacionID: "id-serv"},
		{Clasificacion: "Deposito de Clientes", ClasificacionID: "id-dep"}, // repetida
	}
	if got := nombreArchivoMovimientos("Valle de Paz", f, movs); got != "VDP Serv + Dep Agosto "+hoy+".xlsx" {
		t.Errorf("nombre = %q, quiere «VDP Serv + Dep Agosto %s.xlsx»", got, hoy)
	}

	// Y al revés: eligiendo Depósito primero, sale «Dep + Serv».
	alRevés := FiltrosMovimientos{Periodo: "2026-08", ClasificacionIDs: []string{"id-dep", "id-serv"}}
	if got := nombreArchivoMovimientos("Valle de Paz", alRevés, movs); got != "VDP Dep + Serv Agosto "+hoy+".xlsx" {
		t.Errorf("nombre = %q, quiere «VDP Dep + Serv Agosto %s.xlsx»", got, hoy)
	}
}

// El nombre describe lo que el archivo TRAE: una clasificación que se filtró pero no tuvo
// movimientos en el mes no se menciona.
func TestNombreSoloNombraLoQueSalio(t *testing.T) {
	f := FiltrosMovimientos{Periodo: "2026-08", ClasificacionIDs: []string{"id-asoc", "id-dep", "id-sin-movs"}}
	movs := []MovimientoExport{
		{Clasificacion: "Asociaciones", ClasificacionID: "id-asoc"},
		{Clasificacion: "Deposito de Clientes", ClasificacionID: "id-dep"},
	}
	got := nombreArchivoMovimientos("Valle de Paz", f, movs)
	if got != "VDP Asoc + Dep Agosto "+AhoraCR().Format("02012006")+".xlsx" {
		t.Errorf("nombre = %q; la tercera clasificación no tuvo movimientos y no debería aparecer", got)
	}

	// Sin filtro de clasificación, «Completo» aunque los movimientos traigan partidas.
	sinFiltro := nombreArchivoMovimientos("Valle de Paz", FiltrosMovimientos{Periodo: "2026-08"}, movs)
	if !strings.Contains(sinFiltro, "Completo") {
		t.Errorf("sin filtrar por clasificación el nombre debería decir Completo: %q", sinFiltro)
	}
}

// La hoja de trabajo filtra de a UNA clasificación (el campo singular): también tiene que nombrarse.
func TestNombreConLaClasificacionSingular(t *testing.T) {
	f := FiltrosMovimientos{Periodo: "2026-08", ClasificacionID: "id-dep"}
	movs := []MovimientoExport{{Clasificacion: "Deposito de Clientes", ClasificacionID: "id-dep"}}
	if got := nombreArchivoMovimientos("Valle de Paz", f, movs); !strings.Contains(got, "VDP Dep Agosto") {
		t.Errorf("nombre = %q, quiere «VDP Dep Agosto …»", got)
	}
}
