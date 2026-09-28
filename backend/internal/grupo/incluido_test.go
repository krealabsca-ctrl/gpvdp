package grupo

import (
	"context"
	"os"
	"strings"
	"testing"
)

// Un estado de cuenta importado dos veces se corrige marcando los movimientos de la cuenta
// equivocada con `movimiento_bancario.incluido = false`. Esa marca es la compuerta que el resto del
// ERP ya respeta; las tres consultas de dinero de este paquete no la miraban y seguían contando la
// plata duplicada en la pantalla que mira el Director Financiero.
//
// Estas guardas leen el propio SQL. Es inusual, y acá es lo correcto por la misma razón que en
// seed/fuente_test.go: lo que hay que fijar no es un valor de retorno sino una condición DENTRO de
// una consulta, y probar eso de verdad pide un PostgreSQL que este paquete no levanta
// (service_test.go corre contra un fakeRepo, así que el SQL real no se ejercita nunca). La prueba
// que sí mide los montos contra la base está en zz_probe_incluido_test.go, pero se omite sin
// PROBE_DSN: sin esta red, el filtro se puede borrar en un refactor y nadie se enteraría hasta que
// el consolidado vuelva a contar dos veces.

// cuerpoDeConsulta devuelve el cuerpo de un método de pgRepository leyendo el archivo fuente, ya sin
// comentarios SQL: así una guarda no se puede dar por satisfecha con un `--` que solo HABLA del
// filtro.
func cuerpoDeConsulta(t *testing.T, metodo string) string {
	t.Helper()
	b, err := os.ReadFile("repository.go")
	if err != nil {
		t.Fatalf("leer repository.go: %v", err)
	}
	src := string(b)
	firma := "func (r *pgRepository) " + metodo + "("
	i := strings.Index(src, firma)
	if i < 0 {
		t.Fatalf("no se encontró %s en repository.go", firma)
	}
	resto := src[i:]
	if j := strings.Index(resto, "\nfunc "); j > 0 {
		resto = resto[:j]
	}

	var sinComentarios strings.Builder
	for _, linea := range strings.Split(resto, "\n") {
		if k := strings.Index(linea, "--"); k >= 0 {
			linea = linea[:k]
		}
		sinComentarios.WriteString(linea)
		sinComentarios.WriteString("\n")
	}
	return sinComentarios.String()
}

// Las tres consultas del consolidado suman o cuentan dinero que tiene que cuadrar: el EBITDA del
// grupo sale de ellas. Ninguna puede contar un movimiento excluido.
func TestLasTresConsultasDeDineroExcluyenLoNoIncluido(t *testing.T) {
	for _, metodo := range []string{"ResumenPorEmpresa", "OperacionesEntreEmpresas", "PartidasDelGrupo"} {
		t.Run(metodo, func(t *testing.T) {
			if !strings.Contains(cuerpoDeConsulta(t, metodo), "m.incluido") {
				t.Errorf("%s lee movimiento_bancario sin filtrar `incluido`: una importación duplicada "+
					"corregida se sigue contando en el consolidado del grupo", metodo)
			}
		})
	}
}

// En ResumenPorEmpresa el filtro va DENTRO de cada FILTER de monto, y en ningún otro lado. Las dos
// colocaciones que parecen naturales rompen algo distinto, y las dos ya se probaron contra la base:
//
//   - en el WHERE: el LEFT JOIN se degrada a INNER y la empresa sin movimientos en el período
//     DESAPARECE del consolidado en silencio, sin SinDatos y sin aviso;
//   - en el ON del LEFT JOIN: `count(m.id)` también queda filtrado, y de ese conteo sale SinDatos.
//     Una empresa cuyo único archivo del mes fue el duplicado quedaría con «no tiene ningún
//     movimiento en el período» — mentira que además manda a re-importar el archivo revertido.
func TestEnResumenPorEmpresaElFiltroVaeEnCadaFilterDeMonto(t *testing.T) {
	q := cuerpoDeConsulta(t, "ResumenPorEmpresa")

	// (a) Todo SUM de dinero filtra.
	const sumador = "SUM(m.monto_crc)"
	sumas := strings.Count(q, sumador)
	if sumas == 0 {
		t.Fatal("cambió la forma de la consulta: ya no se reconoce ningún SUM(m.monto_crc)")
	}
	for resto, i := q, 0; ; i++ {
		k := strings.Index(resto, sumador)
		if k < 0 {
			break
		}
		cola := resto[k+len(sumador):]
		if !strings.HasPrefix(cola, " FILTER (WHERE m.incluido") {
			t.Errorf("el SUM(m.monto_crc) nº %d no arranca su FILTER con `m.incluido`: ese monto "+
				"está sumando la importación duplicada", i+1)
		}
		resto = cola
	}

	// (b) El JOIN no filtra: si filtrara, el conteo mentiría.
	posJoin := strings.Index(q, "LEFT JOIN movimiento_bancario")
	posJoinCla := strings.Index(q, "LEFT JOIN clasificacion")
	if posJoin < 0 || posJoinCla < posJoin {
		t.Fatal("cambió la forma de la consulta: ya no se reconocen los LEFT JOIN")
	}
	if strings.Contains(q[posJoin:posJoinCla], "m.incluido") {
		t.Error("`m.incluido` no puede ir en el ON del LEFT JOIN de movimiento_bancario: filtra también " +
			"count(m.id), de donde sale SinDatos, y una empresa con el mes entero excluido pasaría por " +
			"«no tiene ningún movimiento» — que manda a re-importar el duplicado")
	}

	// (c) El WHERE tampoco: ahí el LEFT JOIN se vuelve INNER.
	posWhere := strings.Index(q, "WHERE e.id = ANY(")
	if posWhere < 0 {
		t.Fatal("cambió la forma de la consulta: ya no se reconoce el WHERE por empresa")
	}
	if strings.Contains(q[posWhere:], "m.incluido") {
		t.Error("`m.incluido` en el WHERE degrada el LEFT JOIN a INNER y borra del consolidado a toda " +
			"empresa sin movimientos en el período")
	}

	// (d) Y los excluidos se publican, que es lo que evita que el número baje sin explicación.
	if !strings.Contains(q, "count(m.id) FILTER (WHERE NOT m.incluido)") {
		t.Error("falta el conteo de excluidos: sin él la pantalla no puede decir por qué bajó el total")
	}
}

// Las dos consultas que arrancan con FROM/JOIN directo sobre movimiento_bancario sí filtran en el
// WHERE: ahí no hay LEFT JOIN que degradar y un movimiento excluido simplemente no tiene que estar.
func TestEnLasConsultasInnerElFiltroVaEnElWhere(t *testing.T) {
	for _, metodo := range []string{"OperacionesEntreEmpresas", "PartidasDelGrupo"} {
		t.Run(metodo, func(t *testing.T) {
			q := cuerpoDeConsulta(t, metodo)
			posWhere := strings.Index(q, "WHERE m.empresa_id = ANY(")
			posFiltro := strings.Index(q, "AND m.incluido")
			if posWhere < 0 {
				t.Fatal("cambió la forma de la consulta: ya no se reconoce el WHERE por empresa")
			}
			if posFiltro < posWhere {
				t.Errorf("%s: `AND m.incluido` tiene que ir en el WHERE que ya acota empresa y período", metodo)
			}
		})
	}
}

// Un mes cuyo único archivo fue el duplicado, ya revertido, NO puede presentarse como «no hay
// movimientos»: ese mensaje manda a alguien a re-importar justo lo que se acaba de corregir.
//
// Los dos casos aportan cero al total, pero piden acciones opuestas, así que la vista tiene que
// separarlos. Y el que tiene todo excluido tampoco puede pasar por confiable: pctClasificado
// devuelve 100 % cuando el total es cero, o sea que sin esta distinción la fila saldría en verde
// respaldando un cero que no sostiene ningún dato.
func TestUnMesEnteroExcluidoNoSePresentaComoMesVacio(t *testing.T) {
	repo := &fakeRepo{
		visibles: []EmpresaVisible{{ID: "e-1", Nombre: "Con todo excluido"}, {ID: "e-2", Nombre: "Nunca cargó"}},
		total:    2,
		filas: []FilaEmpresa{
			// La importación duplicada, ya revertida: hay movimientos, no hay plata.
			{EmpresaID: "e-1", Empresa: "Con todo excluido", IngresosCRC: "0", GastosCRC: "0",
				NeutroCRC: "0", Movimientos: 12, Excluidos: 12, SinClasificarCRC: "0", PctClasificado: "100.0"},
			// El mes que de verdad está vacío.
			{EmpresaID: "e-2", Empresa: "Nunca cargó", IngresosCRC: "0", GastosCRC: "0",
				NeutroCRC: "0", Movimientos: 0, Excluidos: 0, SinClasificarCRC: "0", PctClasificado: "100.0"},
		},
	}

	res, err := nuevo(repo).Resumen(context.Background(), "u-1", "DIRECTOR_FINANCIERO", "2026-09")
	if err != nil {
		t.Fatalf("Resumen: %v", err)
	}

	conExcluido, vacia := res.Empresas[0], res.Empresas[1]

	// Las dos aportan cero, así que las dos son «sin datos»: un cero sin nada que lo respalde no
	// puede pasar por confiable solo porque el porcentaje de un conjunto vacío dé 100 %.
	if !conExcluido.SinDatos {
		t.Error("la empresa con el mes entero excluido tiene que quedar marcada sin datos: su total es " +
			"cero y no hay ni un movimiento que lo respalde")
	}
	if conExcluido.Confiable {
		t.Error("una empresa sin un solo movimiento que cuente no puede salir como confiable al 100 %")
	}
	if !vacia.SinDatos || vacia.Confiable {
		t.Error("la empresa sin movimientos tiene que seguir marcada sin datos y no confiable")
	}

	// Pero el AVISO tiene que distinguirlas, porque la acción que piden es opuesta.
	if !strings.Contains(res.Aviso, "Con todo excluido") || !strings.Contains(res.Aviso, "excluido del cuadre") {
		t.Errorf("el aviso no explica que lo cargado está excluido: %q", res.Aviso)
	}
	if !strings.Contains(res.Aviso, "no hay nada que volver a importar") {
		t.Errorf("el aviso tiene que cortar el reflejo de re-importar, que recrearía el duplicado: %q", res.Aviso)
	}
	if strings.Contains(res.Aviso, "Con todo excluido no tiene ningún movimiento") {
		t.Errorf("el aviso le dice a la empresa con datos excluidos que no tiene movimientos: %q", res.Aviso)
	}
	if !strings.Contains(res.Aviso, "Nunca cargó no tiene ningún movimiento") {
		t.Errorf("el aviso perdió el caso del mes de verdad vacío: %q", res.Aviso)
	}
}
