package bancos

// Departamento y sede: de dónde sale el «quién gastó» y el «dónde», con UNA sola definición.
//
// El valor efectivo de un movimiento se resuelve en CASCADA, del más específico al más general:
//
//  1. La excepción escrita en el propio movimiento (`movimiento_bancario.departamento_id`).
//  2. El departamento de la factura de CxP con la que se concilió (la huella `CXP-`).
//  3. El departamento POR DEFECTO de su partida (`clasificacion.departamento_id`).
//  4. Nada: «sin asignar», que es un estado legítimo y visible.
//
// **Por qué se resuelve al leer y no se copia al movimiento.** Copiar el valor a cada movimiento
// obligaría, cada vez que alguien corrige el default de una partida, a un barrido que actualice
// miles de filas —y a decidir si ese barrido pisa las excepciones—. Resolviendo al leer, poner el
// departamento en una partida corrige su historia completa en el mismo instante y sin tocar un solo
// movimiento. Es la diferencia entre configurar y reprocesar.
//
// El costo es un LEFT JOIN más por consulta, y está medido: son índices por clave primaria.
//
// Igual que con la naturaleza del EBITDA (ver naturaleza.go), estas expresiones viven en UN lugar
// para que el reporte por departamento, el presupuesto y el detalle de movimientos no puedan
// discrepar sobre a quién le toca un gasto.

// joinDimensiones son los LEFT JOIN que necesitan las expresiones de abajo. Asume que la consulta
// tiene `m` (movimiento_bancario) y `cl` (clasificacion, ya unida por m.clasificacion_id).
//
// `dcxp` es la factura de CxP conciliada con el movimiento: puede no haber ninguna.
const joinDimensiones = `
	LEFT JOIN documento_cxp dcxp ON dcxp.id = m.documento_cxp_id
	LEFT JOIN departamento dep ON dep.id = COALESCE(m.departamento_id, dcxp.departamento_id, cl.departamento_id)
	LEFT JOIN sede sd ON sd.id = COALESCE(m.sede_id, cl.sede_id)`

// joinClasifParaDimensiones es el LEFT JOIN de la partida, que `joinDimensiones` da por hecho.
// Va aparte porque varias consultas ya lo tienen por su cuenta.
const joinClasifParaDimensiones = `LEFT JOIN clasificacion cl ON cl.id = m.clasificacion_id`

const (
	// sqlDepartamentoEfectivoID es el id del departamento que le toca al movimiento (o NULL).
	sqlDepartamentoEfectivoID = `COALESCE(m.departamento_id, dcxp.departamento_id, cl.departamento_id)`

	// sqlSedeEfectivaID es el id de la sede que le toca al movimiento (o NULL).
	//
	// La factura de CxP no aporta sede: el documento de CxP no tiene esa dimensión. Si algún día la
	// tuviera, se agrega en el medio de este COALESCE y todas las pantallas la heredan juntas.
	sqlSedeEfectivaID = `COALESCE(m.sede_id, cl.sede_id)`

	// sqlDepartamentoNombre / sqlSedeNombre: el nombre para mostrar. «(sin asignar)» y no cadena
	// vacía ni NULL: una fila en blanco en un reporte se lee como un error de la pantalla, cuando en
	// realidad es un hecho —ese gasto no tiene dueño declarado— y hay que poder verlo y sumarlo.
	sqlDepartamentoNombre = `COALESCE(dep.nombre, '(sin asignar)')`
	sqlSedeNombre         = `COALESCE(sd.nombre, '(sin asignar)')`

	// sqlOrigenDimension explica DE DÓNDE salió el departamento de esa fila. Sin esto, alguien ve
	// «Logística» y no sabe si lo escribió una persona en ese movimiento o si lo heredó de la
	// partida; y sin saberlo no puede corregirlo en el lugar correcto.
	sqlOrigenDimension = `CASE
	                        WHEN m.departamento_id IS NOT NULL THEN 'MOVIMIENTO'
	                        WHEN dcxp.departamento_id IS NOT NULL THEN 'FACTURA'
	                        WHEN cl.departamento_id IS NOT NULL THEN 'PARTIDA'
	                        ELSE 'SIN_ASIGNAR' END`
)

// Orígenes posibles del departamento de un movimiento.
const (
	OrigenDimMovimiento = "MOVIMIENTO"  // lo escribió una persona en este movimiento
	OrigenDimFactura    = "FACTURA"     // lo heredó de la factura de CxP conciliada
	OrigenDimPartida    = "PARTIDA"     // lo heredó del default de su clasificación
	OrigenDimSinAsignar = "SIN_ASIGNAR" // nadie lo declaró
)

// EtiquetaOrigenDimension explica el origen en palabras, para la pantalla.
func EtiquetaOrigenDimension(o string) string {
	switch o {
	case OrigenDimMovimiento:
		return "asignado a este movimiento"
	case OrigenDimFactura:
		return "heredado de la factura de CxP"
	case OrigenDimPartida:
		return "heredado de la partida"
	default:
		return "sin asignar"
	}
}
