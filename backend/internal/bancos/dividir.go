package bancos

// Dividir un movimiento entre partidas (mig 0094).
//
// «Del recibo de luz este mes se pagó 1.400.000, pero de luz normal eran 1.000.000, y esos 400.000
// corresponden a un proyecto especial.» El hecho bancario es uno; la atribución, dos.

import (
	"errors"

	"github.com/shopspring/decimal"
)

var (
	// ErrMovimientoNoDivisible: el movimiento no está en condiciones de dividirse. Lo devuelve el
	// repositorio cuando el UPDATE no encuentra la fila, que a esa altura solo puede ser una
	// carrera: el service ya descartó antes las causas conocidas con su mensaje propio.
	ErrMovimientoNoDivisible = errors.New("bancos: ese movimiento ya no se puede dividir; recargá y volvé a intentar")
	// ErrMontoDivisionInvalido: lo que se aparta tiene que ser mayor que cero.
	ErrMontoDivisionInvalido = errors.New("bancos: el monto a separar tiene que ser mayor que cero")
	// ErrMontoDivisionExcede: apartar todo o más de lo que hay no es dividir.
	//
	// Apartar el monto COMPLETO dejaría el movimiento principal en cero, que no es una división
	// sino una reclasificación disfrazada —y para eso ya está clasificar—.
	ErrMontoDivisionExcede = errors.New("bancos: el monto a separar tiene que ser menor que el del movimiento; si querés moverlo entero, cambiale la clasificación")
	// ErrParteNoDivisible: una parte no se vuelve a dividir.
	//
	// Es una restricción de un nivel a propósito: con partes de partes, «padre + hijos» deja de
	// ser una suma y pasa a ser un árbol que hay que recorrer, y la invariante —que es lo único
	// que protege la plata— se vuelve mucho más difícil de comprobar. Para repartir en tres, se
	// divide dos veces el principal.
	ErrParteNoDivisible = errors.New("bancos: esto ya es una parte de otro movimiento; dividí el movimiento principal otra vez")
	// Acá vivía ErrDivisionConCxP: «no se divide lo enlazado a CxP». Se quitó el 2-nov-2026 con la
	// restricción, en vez de dejarlo declarado sin que nadie lo devuelva: un error muerto cuyo
	// texto afirma una regla que ya no rige es peor que ninguno —el próximo que lo lea va a creer
	// que la restricción sigue—. El porqué del cambio está en PuedeDividirse.
	//
	// ErrExclusionConCxP, en cambio, SIGUE vigente y es otra cosa: excluir saca la plata de los
	// libros y deja la factura conciliada contra algo que ya no está. Repartir no saca nada.
	// ErrDivisionSinClasificacion: la parte que se aparta necesita su propia partida, que es toda
	// la razón de separarla.
	ErrDivisionSinClasificacion = errors.New("bancos: decí a qué clasificación va la parte que separás")
	// ErrParteNoEncontrada: no existe esa parte, o ya se deshizo.
	ErrParteNoEncontrada = errors.New("bancos: no se encontró esa parte")
	// ErrMovimientoExcluidoNoSeDivide: lo que está fuera de los libros no se reparte.
	ErrMovimientoExcluidoNoSeDivide = errors.New("bancos: ese movimiento está fuera de los libros; devolvelo antes de dividirlo")
)

// DivisionInput es lo que se aparta y a qué partida va.
type DivisionInput struct {
	// Monto en la moneda del movimiento, y MontoCRC su equivalente en colones. Los dos porque el
	// movimiento lleva los dos, y recalcular uno a partir del otro acá sería una segunda fórmula
	// de conversión que puede discrepar de la del ERP.
	Monto    decimal.Decimal
	MontoCRC decimal.Decimal

	ConceptoID      string
	ClasificacionID string
	DepartamentoID  string
	SedeID          string
	Motivo          string
}

// ── La huella del reparto ───────────────────────────────────────────────────────────────────
//
// EL PROBLEMA QUE RESUELVE: una parte apartada se veía IDÉNTICA a un movimiento normal. La
// relación padre-hijo existía desde la 0094 y era sólida, pero solo se veía abriendo el panel
// «dividir entre partidas» de esa fila concreta — o sea, sabiendo de antemano lo que se iba a
// encontrar. Y en el .xlsx era peor: dos caras del mismo reparto salen con la misma fecha, el
// mismo consecutivo y la misma descripción, así que contra el estado de cuenta se leen como un
// duplicado.
//
// POR QUÉ UN CÓDIGO Y NO SOLO UNA MARCA: una marca dice «esta fila es parte de algo»; un código
// NOMBRA ese algo. Con él, las caras de un reparto se reconocen entre sí de un vistazo, se puede
// buscar y se puede mencionar en un correo. Es lo mismo que hizo la huella `CXP-` con los pagos.

// prefijoReparto encabeza el código. Distinto de `CXP-` a propósito: los dos van a convivir en la
// misma pantalla y confundirlos llevaría a buscar un pago donde hay un reparto.
const prefijoReparto = "DIV-"

// CodigoDeReparto deriva el código visible desde el id del movimiento PRINCIPAL.
//
// SE DERIVA, NO SE GUARDA: todas las caras de un reparto apuntan al mismo padre, así que el
// padre ya es el identificador del grupo. Una columna nueva sería un segundo lugar donde la
// verdad puede quedar distinta de la relación real.
//
// Ocho dígitos y no doce: un código que se transcribe a mano se transcribe mal, y uno errado
// devuelve CERO resultados igual que uno inexistente. Ocho alcanzan para distinguir repartos
// dentro de una empresa y se copian con el botón, no a ojo.
func CodigoDeReparto(principalID string) string {
	hex := make([]rune, 0, 8)
	for _, r := range principalID {
		if r == '-' {
			continue
		}
		if r >= 'a' && r <= 'z' {
			r -= 32
		}
		hex = append(hex, r)
		if len(hex) == 8 {
			break
		}
	}
	if len(hex) < 8 {
		return ""
	}
	return prefijoReparto + string(hex)
}

// DivisionDeMovimiento es la huella que lleva una fila que participa de un reparto.
//
// Viaja con `omitempty`, igual que PagoCxP y SelloExterno: ausente significa «este movimiento no
// se dividió ni es parte de nada», que es el caso de casi todos.
type DivisionDeMovimiento struct {
	// Codigo identifica el reparto completo. Lo comparten TODAS sus caras.
	Codigo string `json:"codigo"`
	// PrincipalID es el movimiento original, para poder ir a él desde cualquier parte.
	PrincipalID string `json:"principal_id"`
	// EsParte distingue las dos caras de la huella: true = a esta fila la apartaron de otra;
	// false = esta es la original y se le apartó algo. La frase que lee el usuario es distinta en
	// cada caso, y un solo booleano mal leído la invierte.
	EsParte bool `json:"es_parte"`
	// Partes es cuántas caras VIVAS tiene el reparto, contando el principal.
	Partes int `json:"partes"`
	// MontoOriginal es lo que trajo el banco, antes de repartirlo. Es el número que permite
	// cruzar la fila contra el estado de cuenta sin abrir nada.
	MontoOriginal string `json:"monto_original"`
}

// huellaDeReparto arma la huella de una fila, o nil si no participa de ningún reparto.
//
// `divididoDe` vacío significa que esta fila NO es una parte; entonces solo lleva huella si es un
// principal al que se le apartó algo, y eso lo dice `montoOriginal`. Separar las dos condiciones
// importa: sin la segunda, el movimiento original de un reparto saldría sin marca y la pantalla
// mostraría partes huérfanas, sin nada que las agrupe.
func huellaDeReparto(movimientoID, divididoDe, montoOriginal string, partes int) *DivisionDeMovimiento {
	esParte := divididoDe != ""
	if !esParte && (montoOriginal == "" || montoOriginal == "0") {
		return nil
	}
	principal := movimientoID
	if esParte {
		principal = divididoDe
	}
	return &DivisionDeMovimiento{
		Codigo:        CodigoDeReparto(principal),
		PrincipalID:   principal,
		EsParte:       esParte,
		Partes:        partes,
		MontoOriginal: montoOriginal,
	}
}

// ParteMovimiento es una de las caras de un movimiento dividido.
type ParteMovimiento struct {
	ID string `json:"id"`
	// EsPrincipal distingue el movimiento original de las partes que se le apartaron.
	EsPrincipal   bool   `json:"es_principal"`
	Monto         string `json:"monto"`
	EsDebito      bool   `json:"es_debito"`
	Concepto      string `json:"concepto"`
	Clasificacion string `json:"clasificacion"`
	Motivo        string `json:"motivo"`
	Incluido      bool   `json:"incluido"`
}

// Division es el reparto completo de un movimiento: el hecho bancario y sus partes.
type Division struct {
	MovimientoID string `json:"movimiento_id"`
	// Codigo nombra al reparto («DIV-A3F2B1C4») y es el MISMO que sale en la parrilla y en la
	// columna «Reparto» del .xlsx. Lo manda el servidor en vez de derivarlo la pantalla: dos
	// definiciones del mismo formato terminan divergiendo, y un código que no coincide entre el
	// Excel y la pantalla no sirve para cruzar —que es lo único para lo que existe—.
	Codigo string `json:"codigo"`
	// El hecho bancario, que es UNO y no se reparte: la misma fecha, el mismo consecutivo y la
	// misma descripción para todas las partes.
	Fecha       string `json:"fecha"`
	Documento   string `json:"documento"`
	Descripcion string `json:"descripcion"`

	Partes []ParteMovimiento `json:"partes"`
	// MontoAntesDeDividir es lo que trajo el banco; SumaDeLasPartes, lo que suman las caras hoy.
	// Los dos viajan JUNTOS para que la pantalla pueda mostrar la invariante en vez de prometerla:
	// si alguna vez difieren, se ve.
	MontoAntesDeDividir string `json:"monto_antes_de_dividir"`
	SumaDeLasPartes     string `json:"suma_de_las_partes"`
}

// Cuadra dice si el reparto sigue sumando lo que trajo el banco.
//
// Es la invariante del módulo y por eso se contesta, no se asume. Un reparto que no cuadra es
// plata inventada o perdida, y tiene que poder verse en la pantalla.
func (d Division) Cuadra() bool {
	if d.MontoAntesDeDividir == "" || d.SumaDeLasPartes == "" {
		return true // sin dividir: no hay nada que cuadrar
	}
	antes, err1 := decimal.NewFromString(d.MontoAntesDeDividir)
	suma, err2 := decimal.NewFromString(d.SumaDeLasPartes)
	if err1 != nil || err2 != nil {
		return false
	}
	if antes.IsZero() {
		return true
	}
	return antes.Equal(suma)
}

// MovimientoDivisible es lo que hace falta saber antes de dividir.
type MovimientoDivisible struct {
	ID           string `json:"id"`
	Monto        string `json:"monto"`
	MontoCRC     string `json:"monto_crc"`
	Descripcion  string `json:"descripcion"`
	Incluido     bool   `json:"incluido"`
	EsParte      bool   `json:"es_parte"`
	EnlazadoACxP bool   `json:"enlazado_a_cxp"`
}

// PuedeDividirse devuelve el motivo por el que NO se puede, o nil.
//
// Cada causa tiene su propio error y su propia frase: «no se puede» a secas obliga a adivinar, y
// cada una tiene un arreglo distinto —devolverlo a los libros, dividir el principal, o bajar el
// monto—.
//
// ── POR QUÉ ESTAR ENLAZADO A CxP YA NO LO IMPIDE (2-nov-2026) ───────────────────────────────
//
// Porque el enlace y el reparto hablan de cosas distintas: el enlace dice QUÉ FACTURA se pagó, y
// el reparto dice A QUÉ PARTIDA va ese gasto. Un débito que paga una factura de un proveedor
// puede corresponder en parte a otra empresa del grupo, y repartirlo no cambia un colón de lo que
// se le pagó al proveedor.
//
// La restricción existía por un riesgo que se midió y resultó ser de PRESENTACIÓN, no de cuadre:
// ningún proceso compara el monto del movimiento contra el de la factura —la conciliación enlaza
// por la huella `CXP-` que viaja en la descripción—, y el único lugar donde el monto reducido se
// notaba era el expediente de la factura, que ahora suma las caras del reparto. El enlace queda
// en el PRINCIPAL: las partes no lo heredan, así que no hay forma de que una factura termine
// apuntada desde dos filas.
//
// Lo que sostiene todo esto es la invariante de siempre: las caras suman lo que trajo el banco,
// que es exactamente lo que se le pagó a la factura.
func (m MovimientoDivisible) PuedeDividirse() error {
	switch {
	case !m.Incluido:
		return ErrMovimientoExcluidoNoSeDivide
	case m.EsParte:
		return ErrParteNoDivisible
	}
	return nil
}
