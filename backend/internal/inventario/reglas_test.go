package inventario

// Pruebas de las REGLAS del inventario: las decisiones que no dependen de la base.
//
// Lo que se fija acá son las cuatro que, si cambian sin querer, hacen que el módulo mienta:
// el signo de un movimiento, el semáforo de existencia, la lectura de rotación y la cantidad
// sugerida a pedir. El comportamiento contra PostgreSQL (transacciones, índices parciales) se
// verifica E2E: un doble de prueba no tiene índices únicos ni CHECK.

import (
	"strings"
	"testing"
)

func TestElSignoDeUnMovimientoViveEnUnSoloLugar(t *testing.T) {
	t.Parallel()
	// Si el signo se decidiera en cada consulta, dos pantallas podrían mostrar existencias
	// distintas del mismo artículo. `sumaAlStock` y `sqlSignoMovimiento` tienen que coincidir.
	suman := []string{MovEntrada, MovTrasladoEntrada, MovAjusteMas}
	restan := []string{MovSalida, MovTrasladoSalida, MovAjusteMenos, MovBaja, MovDevolucion}

	for _, tipo := range suman {
		if !sumaAlStock(tipo) {
			t.Errorf("%s debería sumar al stock", tipo)
		}
		if !strings.Contains(sqlSignoMovimiento, "'"+tipo+"'") {
			t.Errorf("%s suma en Go pero no está en la expresión SQL: los dos números se van a separar", tipo)
		}
	}
	for _, tipo := range restan {
		if sumaAlStock(tipo) {
			t.Errorf("%s no debería sumar al stock", tipo)
		}
		if strings.Contains(sqlSignoMovimiento, "'"+tipo+"'") {
			t.Errorf("%s resta en Go pero el SQL lo suma", tipo)
		}
	}
}

func TestSinMinimoDefinidoNoHaySemaforo(t *testing.T) {
	t.Parallel()
	// Decir «en rango» sobre un mínimo que nadie fijó es afirmar algo que no se sabe.
	if got := estadoDeNivel(5, 0, 0); got != NivelSinNivel {
		t.Errorf("sin mínimo ni máximo el estado debe ser SIN_NIVEL, dio %s", got)
	}
	if got := estadoDeNivel(0, 0, 0); got != NivelSinNivel {
		t.Errorf("cero existencia sin mínimo sigue siendo SIN_NIVEL, dio %s", got)
	}
}

func TestElSemaforoAvisaANTESDeLlegarAlMinimo(t *testing.T) {
	t.Parallel()
	// Un artículo justo en el mínimo ya es tarde para pedir si el proveedor tarda dos semanas: por
	// eso hay un tramo de aviso previo (el 125 % del mínimo).
	casos := []struct {
		nombre              string
		hay, minimo, maximo int
		quiere              string
	}{
		{"bajo el mínimo", 4, 8, 20, NivelBajo},
		{"justo en el mínimo todavía avisa", 8, 8, 20, NivelCerca},
		{"un poco arriba del mínimo avisa", 9, 8, 20, NivelCerca},
		{"cómodo", 15, 8, 20, NivelEnRango},
		{"por encima del máximo", 22, 8, 20, NivelSobre},
		{"sin máximo, cómodo", 30, 8, 0, NivelEnRango},
		{"vacío con mínimo", 0, 3, 0, NivelBajo},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got := estadoDeNivel(c.hay, c.minimo, c.maximo); got != c.quiere {
				t.Errorf("hay %d, mín %d, máx %d → %s, se esperaba %s", c.hay, c.minimo, c.maximo, got, c.quiere)
			}
		})
	}
}

func TestLaRotacionSeAnualizaParaPoderCompararRangos(t *testing.T) {
	t.Parallel()
	// Sin anualizar, «6 salidas en un mes» y «6 salidas en un año» darían la misma rotación y una
	// de las dos lecturas estaría muy mal.
	unMes := conLecturaDeRotacion(RotacionArticulo{EnStock: 10, Salidas: 6}, 30)
	unAnio := conLecturaDeRotacion(RotacionArticulo{EnStock: 10, Salidas: 6}, 365)
	if unMes.Rotacion == unAnio.Rotacion {
		t.Fatalf("el mismo número de salidas en un mes y en un año no puede dar la misma rotación (%s)", unMes.Rotacion)
	}
	if unMes.Lectura != RotacionSana {
		t.Errorf("6 salidas al mes sobre 10 en stock es rotación sana, dio %s", unMes.Lectura)
	}
	if unAnio.Lectura != RotacionDetenida {
		t.Errorf("6 salidas al AÑO sobre 10 en stock está detenido, dio %s", unAnio.Lectura)
	}
}

func TestSinSalidasNoSeInventaUnaRotacion(t *testing.T) {
	t.Parallel()
	// Un artículo sin salidas no rota «0 veces»: no hay con qué calcularlo, y decir 0,0× invita a
	// leerlo como un dato medido.
	r := conLecturaDeRotacion(RotacionArticulo{EnStock: 8, Salidas: 0}, 365)
	if r.Lectura != RotacionSinSalidas {
		t.Errorf("lectura = %s, se esperaba SIN_SALIDAS", r.Lectura)
	}
	if r.Rotacion != "" || r.DiasDeStock != "" {
		t.Errorf("sin salidas no debería haber rotación ni días de stock: %q / %q", r.Rotacion, r.DiasDeStock)
	}

	// Y si salió TODO, no hay existencia contra la que medir, y eso no es un problema.
	vacio := conLecturaDeRotacion(RotacionArticulo{EnStock: 0, Salidas: 12}, 365)
	if vacio.Lectura != RotacionSana {
		t.Errorf("salió todo: lectura = %s, se esperaba SANO", vacio.Lectura)
	}
}

func TestElSugeridoCubreLaEntregaSinPasarDelMaximo(t *testing.T) {
	t.Parallel()
	casos := []struct {
		nombre string
		f      SugerenciaPedido
		quiere int
	}{
		{
			"sin consumo, llega al máximo",
			SugerenciaPedido{Hay: 4, Minimo: 8, Maximo: 20, ConsumoSemanal: "0"},
			16,
		},
		{
			"el consumo de la entrega manda cuando es mayor",
			// 5 por semana × 2 semanas = 10, contra 12 para llegar al máximo → gana 12.
			SugerenciaPedido{Hay: 8, Minimo: 8, Maximo: 20, ConsumoSemanal: "5"},
			12,
		},
		{
			"nunca pasa del máximo",
			SugerenciaPedido{Hay: 18, Minimo: 8, Maximo: 20, ConsumoSemanal: "9"},
			2,
		},
		{
			"sin máximo, apunta al mínimo o al consumo",
			SugerenciaPedido{Hay: 2, Minimo: 6, Maximo: 0, ConsumoSemanal: "4"},
			8, // 4 × 2 semanas = 8, mayor que los 4 que faltan para el mínimo.
		},
		{
			"si ya sobra, no pide",
			SugerenciaPedido{Hay: 25, Minimo: 8, Maximo: 20, ConsumoSemanal: "0"},
			0,
		},
		{
			"un consumo ilegible no rompe: cae al faltante",
			SugerenciaPedido{Hay: 4, Minimo: 8, Maximo: 20, ConsumoSemanal: "no es un número"},
			16,
		},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got := sugerirConConsumo(c.f); got != c.quiere {
				t.Errorf("sugerido = %d, se esperaba %d", got, c.quiere)
			}
		})
	}
}

func TestLosMensajesDicenQueHacerYNoSoloQueFallo(t *testing.T) {
	t.Parallel()
	// «No hay existencia» a secas obliga a ir a buscar los números a otra pantalla.
	sin := &SinExistenciaError{Articulo: "Urna mármol", Sede: "Cartago", Hay: 4, Pedido: 99}
	for _, esperado := range []string{"Cartago", "4", "99", "Urna mármol"} {
		if !strings.Contains(sin.Error(), esperado) {
			t.Errorf("el mensaje no dice %q: %s", esperado, sin.Error())
		}
	}

	// El estado tiene que estar en palabras: «DANADA» no es lenguaje de nadie.
	nd := &UnidadNoDisponibleError{Numero: "CF-0412", Estado: EstadoDanada, Quiere: "trasladarla"}
	if !strings.Contains(nd.Error(), "dada de baja por daño") {
		t.Errorf("el estado debería salir en palabras: %s", nd.Error())
	}

	// «No apareció» y «se dañó» son hechos distintos y no pueden compartir la palabra: el contero
	// no comprobó ningún daño, solo que la unidad no estaba.
	if EtiquetaEstado(EstadoNoAparecio) == EtiquetaEstado(EstadoDanada) {
		t.Error("una unidad que no apareció no está dañada: la etiqueta no puede ser la misma")
	}
	if !strings.Contains(EtiquetaEstado(EstadoNoAparecio), "conteo") {
		t.Errorf("la etiqueta debería decir de dónde sale el estado: %s", EtiquetaEstado(EstadoNoAparecio))
	}

	// Y el de sede tiene que decir DÓNDE está, que es lo que permite resolverlo.
	otra := &UnidadEnOtraSedeError{Numero: "CF-0412", Esta: "Sabana", Pedida: "Liberia"}
	if !strings.Contains(otra.Error(), "Sabana") || !strings.Contains(otra.Error(), "Liberia") {
		t.Errorf("el mensaje debería nombrar las dos sedes: %s", otra.Error())
	}
	// En tránsito no tiene sede: el mensaje tiene que seguir teniendo sentido.
	transito := &UnidadEnOtraSedeError{Numero: "CF-0470", Esta: "", Pedida: "Liberia"}
	if !strings.Contains(transito.Error(), "tránsito") {
		t.Errorf("una unidad sin sede está en tránsito y hay que decirlo: %s", transito.Error())
	}
}

func TestElModoDeControlSoloAceptaLosDosQueExisten(t *testing.T) {
	t.Parallel()
	// El modo se concatena en decisiones de negocio y llega del cliente: validarlo es lo que impide
	// que un valor inventado se filtre hasta la base.
	if !modoValido(ModoUnidad) || !modoValido(ModoCantidad) {
		t.Error("los dos modos reales tienen que ser válidos")
	}
	for _, malo := range []string{"", "unidad", "PESO", "UNIDAD ", "CANTIDAD;DROP TABLE inv_unidad"} {
		if modoValido(malo) {
			t.Errorf("%q no debería ser un modo válido", malo)
		}
	}
}

func TestLasFechasSeValidanEnElBorde(t *testing.T) {
	t.Parallel()
	// Una fecha mal escrita tiene que salir como «fecha inválida» y no como error interno del
	// servidor cuando el ::date de Postgres reviente.
	for _, buena := range []string{"2026-08-22", "2025-01-01"} {
		if !reFecha.MatchString(buena) {
			t.Errorf("%q debería ser una fecha válida", buena)
		}
	}
	for _, mala := range []string{"", "22/08/2026", "2026-8-2", "ayer", "2026-08-22T10:00:00"} {
		if reFecha.MatchString(mala) {
			t.Errorf("%q no debería pasar la validación", mala)
		}
	}
}
