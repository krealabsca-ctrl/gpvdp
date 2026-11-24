package bancos

// «Qué ve» una persona: el alcance de «Mi presupuesto» y de «Mi ingreso», desde su ficha.
//
// ── EL PROBLEMA QUE RESUELVE ─────────────────────────────────────────────────────────────────
//
// El permiso dice QUÉ PANTALLA puede abrir; el alcance dice QUÉ DATOS ve. Son dos cosas y vivían
// en lugares distintos: el permiso en Seguridad, las áreas de gasto en CxP › Departamentos —otro
// módulo entero— y las partidas de ingreso en ningún lado, solo por SQL. Para armar a una persona
// había que recorrer tres pantallas, una de ellas de un módulo al que esa persona ni entra.
//
// ── MARCAR UN ÁREA TAMBIÉN DA PODER DE APROBAR EN CxP ────────────────────────────────────────
//
// `departamento_validador` es la MISMA tabla con la que CxP decide quién valida las facturas de
// un departamento. No se duplica el dato a propósito —el responsable del área es el responsable
// del área, y dos listas separadas se desincronizan—, pero la consecuencia es real: dar «ve el
// presupuesto de Mercadeo» convierte a esa persona en validadora de las facturas de Mercadeo. La
// pantalla lo dice con todas las letras; esconderlo sería repartir poder de aprobación sin que
// quien lo reparte se entere.
//
// ── LA GUARDA QUE IMPORTA ────────────────────────────────────────────────────────────────────
//
// `departamento_validador` NO tiene `empresa_id`: cuelga del departamento. Así que el aislamiento
// depende enteramente de comprobar, en cada escritura, que el usuario y el departamento son de la
// empresa del token. Sin esa comprobación, un administrador de una empresa podría darle alcance a
// alguien de otra; es el único camino por el que esto puede cruzar empresas.

import (
	"errors"
)

var (
	// ErrUsuarioFueraDeLaEmpresa: la persona no tiene acceso a la empresa del token. Es la guarda
	// de aislamiento, no una validación cosmética.
	ErrUsuarioFueraDeLaEmpresa = errors.New("bancos: esa persona no tiene acceso a esta empresa")
	// ErrAreaDesconocida: el departamento no existe en esta empresa, o está inactivo.
	ErrAreaDesconocida = errors.New("bancos: una de las áreas no existe en esta empresa")
	// ErrPartidaDesconocida: la clasificación no existe en esta empresa, o está inactiva.
	ErrPartidaDesconocida = errors.New("bancos: una de las partidas no existe en esta empresa")
	// ErrPartidaNoEsDeIngreso: se intentó asignar una partida de gasto al alcance de ingreso.
	// Pasaría desapercibido: la pantalla la nombraría arriba y nunca mostraría una línea, porque
	// el control de ingreso solo mira los conceptos de naturaleza INGRESO.
	ErrPartidaNoEsDeIngreso = errors.New("bancos: solo se pueden asignar partidas de ingreso; esa es de otra naturaleza")
	// ErrRolDeAreaInvalido: el rol en el área solo puede ser TITULAR o SUPLENTE.
	ErrRolDeAreaInvalido = errors.New("bancos: el rol en el área debe ser TITULAR o SUPLENTE")
)

// RolEnArea es cómo participa la persona en el área: quien manda o quien cubre.
const (
	AreaTitular  = "TITULAR"
	AreaSuplente = "SUPLENTE"
)

// RolDeAreaValido dice si el rol es uno de los dos que acepta la base.
func RolDeAreaValido(rol string) bool {
	return rol == AreaTitular || rol == AreaSuplente
}

// AreaAsignada es un área que se le da (o se le quita) a la persona.
type AreaAsignada struct {
	DepartamentoID string `json:"departamento_id" binding:"required,uuid"`
	Rol            string `json:"rol"`
}

// AlcanceDelUsuario es todo lo que una persona ve, en un solo lugar.
type AlcanceDelUsuario struct {
	UsuarioID string `json:"usuario_id"`
	Nombre    string `json:"nombre"`
	Email     string `json:"email"`
	// Areas gobierna «Mi presupuesto» (gasto) Y la validación de facturas en CxP.
	Areas []AreaDelUsuario `json:"areas"`
	// Partidas gobierna «Mi ingreso». Solo de naturaleza INGRESO.
	Partidas []PartidaDelUsuario `json:"partidas"`
	// Los dos permisos que hacen falta para que el alcance sirva de algo. Van acá para que la
	// pantalla pueda avisar «le asignaste áreas pero no puede abrir la pantalla», que es el error
	// que se comete la primera vez y deja a alguien mirando un 403 sin saber por qué.
	PuedeVerPresupuesto bool `json:"puede_ver_presupuesto"`
	PuedeVerIngreso     bool `json:"puede_ver_ingreso"`
}

// NormalizarAreas deja el rol en TITULAR cuando viene vacío y valida el resto.
//
// Vacío → TITULAR porque es el caso normal: casi todo el mundo es titular de su área, y obligar a
// elegir en cada fila sería ruido. Un valor escrito a mano que no sea uno de los dos SÍ se
// rechaza: la base lo rechazaría igual, con un error que no se entiende.
func NormalizarAreas(areas []AreaAsignada) ([]AreaAsignada, error) {
	out := make([]AreaAsignada, 0, len(areas))
	vistos := make(map[string]bool, len(areas))
	for _, a := range areas {
		if a.Rol == "" {
			a.Rol = AreaTitular
		}
		if !RolDeAreaValido(a.Rol) {
			return nil, ErrRolDeAreaInvalido
		}
		// La misma área dos veces en el mismo envío rompería el INSERT por clave duplicada.
		if vistos[a.DepartamentoID] {
			continue
		}
		vistos[a.DepartamentoID] = true
		out = append(out, a)
	}
	return out, nil
}

// SinRepetidos deja una sola vez cada id, conservando el orden en que llegaron.
func SinRepetidos(ids []string) []string {
	out := make([]string, 0, len(ids))
	vistos := make(map[string]bool, len(ids))
	for _, id := range ids {
		if id == "" || vistos[id] {
			continue
		}
		vistos[id] = true
		out = append(out, id)
	}
	return out
}
