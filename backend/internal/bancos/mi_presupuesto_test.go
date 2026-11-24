package bancos

// «Mi presupuesto»: el recorte por área (mig 0124, 16-nov-2026).
//
// ESTA ES LA FUNCIÓN DE SEGURIDAD DE LA PANTALLA y por eso se prueba sin base: lo que tiene que
// resistir es que le entreguen un control con áreas ajenas adentro —que es exactamente lo que le
// llega, porque reusa el cálculo completo de Finanzas— y deje salir solo lo propio.
//
// El defecto que esto evita no da ningún error: un recorte que falla entrega la pantalla completa
// con el rótulo del área de quien la abrió, y nadie lo nota hasta que un jefe pregunta por qué ve
// el gasto de otra gerencia.
//
// Y LO SIN DUEÑO NO ES DE NADIE. Hoy son ₡127,5 M (el 37,9 % del gasto): repartirlo haría a cada
// área más grande de lo que es, y mostrárselo a un jefe sería mostrarle plata de toda la empresa.

import (
	"strings"
	"testing"
)

func controlDeMuestra() ControlPresupuestario {
	return ControlPresupuestario{
		Desde: "2026-10", Hasta: "2026-10",
		Filas: []FilaControl{
			{DepartamentoID: "d-ope", Departamento: "Operaciones", Presupuesto: "50000000.00",
				Gasto: "48115442.00", Estado: CtrlAlerta, Movs: 104},
			{DepartamentoID: "d-fin", Departamento: "Finanzas", Presupuesto: "",
				Gasto: "119414681.00", Estado: CtrlSinPresupuesto, Movs: 239},
			{DepartamentoID: "d-mer", Departamento: "Mercadeo", Presupuesto: "5000000.00",
				Gasto: "4199019.00", Estado: CtrlEnRango, Movs: 15},
			// La fila del gasto sin dueño: sin id, y no es de nadie.
			{DepartamentoID: "", Departamento: "(sin asignar)", Gasto: "127526463.00", Movs: 651},
		},
		TotalGasto: "299255605.00", SinAsignar: "127526463.00", SinAsignarMovs: 651,
		Meses: []SaludMes{{Periodo: "2026-10", PctClasificado: "94.0", Comparable: true}},
		Aviso: "",
	}
}

// SOLO SALEN LAS ÁREAS DE LA PERSONA.
func TestMiPresupuestoNoDejaPasarAreasAjenas(t *testing.T) {
	mp := RecortarAMisAreas(controlDeMuestra(), []AreaDelUsuario{
		{DepartamentoID: "d-mer", Departamento: "Mercadeo", Rol: "TITULAR"},
	})

	if len(mp.Filas) != 1 {
		t.Fatalf("filas = %d; se esperaba solo Mercadeo. Las ajenas: %+v", len(mp.Filas), mp.Filas)
	}
	if mp.Filas[0].Departamento != "Mercadeo" {
		t.Errorf("salió %q", mp.Filas[0].Departamento)
	}
	// Los totales son de SU área, no de la empresa: ₡4,2 M y no ₡299,2 M.
	if mp.TotalGasto != "4199019.00" {
		t.Errorf("total gasto = %s; se esperaba el de Mercadeo (4199019.00), no el de la empresa", mp.TotalGasto)
	}
	if mp.TotalPresupuesto != "5000000.00" {
		t.Errorf("total presupuesto = %s", mp.TotalPresupuesto)
	}
}

// EL GASTO SIN DUEÑO NO VIAJA, ni como fila ni dentro del total.
func TestMiPresupuestoNoMuestraElGastoSinDueno(t *testing.T) {
	// Incluso pidiendo TODAS las áreas reales, la fila sin id queda afuera.
	mp := RecortarAMisAreas(controlDeMuestra(), []AreaDelUsuario{
		{DepartamentoID: "d-ope", Departamento: "Operaciones", Rol: "TITULAR"},
		{DepartamentoID: "d-fin", Departamento: "Finanzas", Rol: "TITULAR"},
		{DepartamentoID: "d-mer", Departamento: "Mercadeo", Rol: "TITULAR"},
	})
	for _, f := range mp.Filas {
		if f.DepartamentoID == "" || strings.Contains(f.Departamento, "sin asignar") {
			t.Errorf("se coló el gasto sin dueño: %+v", f)
		}
	}
	// 48.115.442 + 119.414.681 + 4.199.019 = 171.729.142, SIN los 127,5 M sin dueño.
	if mp.TotalGasto != "171729142.00" {
		t.Errorf("total gasto = %s; se esperaba 171729142.00 (sin el gasto sin dueño)", mp.TotalGasto)
	}
}

// Un área SIN presupuesto se cuenta: es lo que va a ver casi todo el mundo al abrir.
func TestMiPresupuestoCuentaLasAreasSinPresupuesto(t *testing.T) {
	mp := RecortarAMisAreas(controlDeMuestra(), []AreaDelUsuario{
		{DepartamentoID: "d-fin", Departamento: "Finanzas", Rol: "TITULAR"},
		{DepartamentoID: "d-mer", Departamento: "Mercadeo", Rol: "TITULAR"},
	})
	if mp.SinPresupuesto != 1 {
		t.Errorf("sin_presupuesto = %d; Finanzas no tiene monto definido", mp.SinPresupuesto)
	}
	// Y el total de presupuesto solo suma lo que SÍ está definido.
	if mp.TotalPresupuesto != "5000000.00" {
		t.Errorf("total presupuesto = %s; Finanzas no aporta nada", mp.TotalPresupuesto)
	}
}

// Las áreas viajan SIEMPRE, aunque no tengan gasto: «no aparece» y «no gastó» son distintos.
func TestMiPresupuestoNombraMisAreasAunqueNoTenganGasto(t *testing.T) {
	mp := RecortarAMisAreas(controlDeMuestra(), []AreaDelUsuario{
		{DepartamentoID: "d-tec", Departamento: "Tecnología", Rol: "SUPLENTE"},
	})
	if len(mp.Filas) != 0 {
		t.Errorf("filas = %d; Tecnología no tiene gasto en el control", len(mp.Filas))
	}
	if len(mp.Areas) != 1 || mp.Areas[0].Departamento != "Tecnología" {
		t.Errorf("las áreas no viajaron: %+v", mp.Areas)
	}
	// El rol viaja: un suplente tiene que saber que lo está viendo como suplente.
	if mp.Areas[0].Rol != "SUPLENTE" {
		t.Errorf("rol = %q", mp.Areas[0].Rol)
	}
	if mp.TotalGasto != "0.00" {
		t.Errorf("total gasto = %s; sin filas tiene que ser 0,00 y no vacío", mp.TotalGasto)
	}
}

// La salud de clasificación SÍ viaja: no revela movimientos y sin ella un mes a medio clasificar
// se lee como un mes de bajo gasto.
func TestMiPresupuestoConservaLaSaludDelMes(t *testing.T) {
	mp := RecortarAMisAreas(controlDeMuestra(), []AreaDelUsuario{
		{DepartamentoID: "d-mer", Departamento: "Mercadeo", Rol: "TITULAR"},
	})
	if len(mp.Meses) != 1 || mp.Meses[0].PctClasificado != "94.0" {
		t.Errorf("la salud del mes no viajó: %+v", mp.Meses)
	}
}

// Con un alcance vacío NO sale ni una fila. Es la segunda guarda: el servicio ya corta antes, pero
// si alguien reordena eso, acá no pasa nada igual.
func TestMiPresupuestoConAlcanceVacioNoDevuelveNada(t *testing.T) {
	mp := RecortarAMisAreas(controlDeMuestra(), nil)
	if len(mp.Filas) != 0 {
		t.Fatalf("con alcance vacío salieron %d fila(s): el recorte se abrió en vez de cerrarse", len(mp.Filas))
	}
	if mp.TotalGasto != "0.00" {
		t.Errorf("total gasto = %s con alcance vacío", mp.TotalGasto)
	}
	// Y las listas van vacías, no nulas: un nil de Go se serializa como `null` y rompe la pantalla.
	if mp.Filas == nil || mp.Meses == nil {
		t.Error("las listas vinieron nulas")
	}
}
