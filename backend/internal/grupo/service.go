package grupo

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// PermisoLecturaBancos es el permiso que se exige POR EMPRESA para incluirla en el consolidado.
//
// La vista no otorga acceso: suma lo que el usuario ya podía ver de a una. Si mañana el consolidado
// incluye CxP, será con `cxp.ver` por empresa, con la misma regla.
const PermisoLecturaBancos = "bancos.ver"

// umbralConfiable es el porcentaje de clasificación (por MONTO) a partir del cual el proyecto da un
// número por bueno. Es el mismo 90 % que ya usa Bancos: no se inventa un criterio nuevo para el
// consolidado.
const umbralConfiable = 90.0

// rolAdmin es el rol con bypass total. Se compara por string y no se importa de rbac a propósito:
// este paquete no depende de ningún otro módulo. Si el bypass se resolviera distinto acá, un admin
// vería MENOS empresas en el consolidado que entrando de a una, y nadie entendería por qué.
const rolAdmin = "ADMIN"

// topePartidas acota el detalle de partidas. Más de esto no se lee en una pantalla; para explorar
// están las pantallas por empresa.
const topePartidas = 15

var rePeriodo = regexp.MustCompile(`^\d{4}-(0[1-9]|1[0-2])$`)

// Errores de la vista.
var (
	// ErrPeriodoInvalido indica un período mal escrito.
	ErrPeriodoInvalido = errors.New("grupo: el período tiene que ser AAAA-MM")
	// ErrSinEmpresasVisibles indica que el usuario no puede ver ninguna empresa con ese permiso. No
	// es un fallo del servidor: es que no tiene acceso, y hay que decirlo así.
	ErrSinEmpresasVisibles = errors.New("grupo: no tenés acceso de lectura a ninguna empresa, así que no hay nada que consolidar")
)

// Resumen arma la vista consolidada del período.
//
// El orden importa: primero se resuelve QUÉ empresas se pueden ver, y todo lo demás se consulta con
// esa lista. Ninguna consulta de este paquete puede leer una empresa que no pasó por ese filtro.
func (s *Service) Resumen(ctx context.Context, usuarioID, rol, periodo string) (ResumenGrupo, error) {
	if !rePeriodo.MatchString(periodo) {
		return ResumenGrupo{}, fmt.Errorf("%w: %q", ErrPeriodoInvalido, periodo)
	}

	visibles, err := s.repo.EmpresasVisibles(ctx, usuarioID, PermisoLecturaBancos, rol == rolAdmin)
	if err != nil {
		return ResumenGrupo{}, err
	}
	if len(visibles) == 0 {
		return ResumenGrupo{}, ErrSinEmpresasVisibles
	}

	ids := make([]string, len(visibles))
	nombres := map[string]bool{}
	for i, e := range visibles {
		ids[i] = e.ID
		nombres[e.Nombre] = true
	}

	filas, err := s.repo.ResumenPorEmpresa(ctx, ids, periodo)
	if err != nil {
		return ResumenGrupo{}, err
	}
	internas, err := s.repo.OperacionesEntreEmpresas(ctx, ids, periodo)
	if err != nil {
		return ResumenGrupo{}, err
	}
	partidas, err := s.repo.PartidasDelGrupo(ctx, ids, periodo, topePartidas)
	if err != nil {
		return ResumenGrupo{}, err
	}

	// Cuántas hay en total. Si falla, se pierde solo el aviso de las excluidas: no es motivo para
	// negar la vista completa.
	totalEmpresas, err := s.repo.ContarEmpresas(ctx)
	if err != nil {
		s.log.Warn("grupo: no se pudo contar las empresas del sistema", zap.Error(err))
		totalEmpresas = 0
	}

	out := ResumenGrupo{
		Periodo:            periodo,
		Empresas:           filas,
		EntreEmpresas:      internas,
		Partidas:           partidas,
		EmpresasDelSistema: totalEmpresas,
	}

	// Los totales del grupo son la SUMA de las filas, sin eliminar nada: decisión del Director
	// Financiero. Lo intercompañía se informa aparte, no se resta.
	ingresos, gastos, neutro, sinClasif := decimal.Zero, decimal.Zero, decimal.Zero, decimal.Zero
	for i := range out.Empresas {
		f := &out.Empresas[i]
		fi := aDecimal(f.IngresosCRC)
		fg := aDecimal(f.GastosCRC)
		f.EbitdaCRC = fi.Sub(fg).StringFixed(2)
		f.SinDatos = f.Movimientos == 0
		f.Confiable = !f.SinDatos && esConfiable(f.PctClasificado)

		ingresos = ingresos.Add(fi)
		gastos = gastos.Add(fg)
		neutro = neutro.Add(aDecimal(f.NeutroCRC))
		sinClasif = sinClasif.Add(aDecimal(f.SinClasificarCRC))
		out.Movimientos += f.Movimientos
	}
	out.IngresosCRC = ingresos.StringFixed(2)
	out.GastosCRC = gastos.StringFixed(2)
	out.EbitdaCRC = ingresos.Sub(gastos).StringFixed(2)
	out.NeutroCRC = neutro.StringFixed(2)
	out.SinClasificarCRC = sinClasif.StringFixed(2)

	entre, entreEbitda := decimal.Zero, decimal.Zero
	for i := range out.EntreEmpresas {
		o := &out.EntreEmpresas[i]
		// La bandera se deriva acá, en el mismo lugar donde se suma: así el detalle que la pantalla
		// muestra y el total del aviso no pueden contradecirse.
		o.AfectaEbitda = afectaEbitda(o.Naturaleza)
		m := aDecimal(o.MontoCRC)
		entre = entre.Add(m)
		if o.AfectaEbitda {
			entreEbitda = entreEbitda.Add(m)
		}
	}
	out.EntreEmpresasCRC = entre.StringFixed(2)
	out.EntreEmpresasEbitdaCRC = entreEbitda.StringFixed(2)

	// Qué empresas del sistema quedaron afuera. No se listan por nombre las que el usuario no puede
	// ver —eso filtraría información— pero SÍ se dice cuántas son: un total que omite una empresa en
	// silencio se lee como el número del grupo.
	if totalEmpresas > len(visibles) {
		out.Excluidas = []string{fmt.Sprintf("%d empresa(s) del sistema no se incluyen porque no tenés acceso de lectura", totalEmpresas-len(visibles))}
	}
	out.Aviso = avisoDelGrupo(out)
	out.Completo = len(out.Excluidas) == 0 && todasConfiables(out.Empresas)
	return out, nil
}

// avisoDelGrupo explica en una frase por qué el número puede no ser el que se espera.
//
// El orden de las advertencias es el de su gravedad: primero lo que hace que el total no sea del
// grupo y después lo que hace que no sea confiable.
//
// No incluye ningún MONTO a propósito. Formatear plata acá obligaría a repetir en Go las reglas de
// separadores y decimales que el cliente ya aplica a todos los montos del ERP, y las dos versiones
// se separan: el aviso decía «₡1290000» justo arriba de una tabla que decía «₡1 290 000,00». Los
// montos viajan como campos y los formatea quien los muestra.
func avisoDelGrupo(r ResumenGrupo) string {
	var partes []string

	if len(r.Excluidas) > 0 {
		partes = append(partes, r.Excluidas[0])
	}

	// Una empresa sin un solo movimiento en el período no es una empresa «mal clasificada»: es que
	// no hay datos, y decir «0 %» haría pensar que alguien no clasificó. Va en su propia frase.
	var vacias, flojas []string
	for _, f := range r.Empresas {
		switch {
		case f.SinDatos:
			vacias = append(vacias, f.Empresa)
		case !f.Confiable:
			flojas = append(flojas, fmt.Sprintf("%s al %s %%", f.Empresa, f.PctClasificado))
		}
	}
	switch {
	// Si están TODAS vacías, nombrarlas una por una es ruido: lo que pasa es que el período todavía
	// no tiene nada cargado, y eso se dice en una frase.
	case len(vacias) > 0 && len(vacias) == len(r.Empresas):
		partes = append(partes, "el período todavía no tiene movimientos cargados en ninguna empresa, así que todos los totales son cero")
	case len(vacias) > 0:
		partes = append(partes, fmt.Sprintf("%s no tiene%s ningún movimiento en el período, así que aporta%s cero al total",
			enumerar(vacias), plural(len(vacias)), plural(len(vacias))))
	}
	if len(flojas) > 0 {
		partes = append(partes, fmt.Sprintf("%s no llega%s al 90 %% clasificado, así que su aporte al total es parcial",
			enumerar(flojas), plural(len(flojas))))
	}

	if len(partes) == 0 {
		return ""
	}
	return strings.Join(partes, " · ") + "."
}

func plural(n int) string {
	if n > 1 {
		return "n"
	}
	return ""
}

// enumerar arma una lista en español: «A», «A y B», «A, B y C». Un Join con " y " daba
// «Coopeprofa y Memorial Pets y Valle de Paz», que se lee como un error de la máquina justo en la
// frase que tiene que hacerse creer.
func enumerar(xs []string) string {
	switch len(xs) {
	case 0:
		return ""
	case 1:
		return xs[0]
	default:
		return strings.Join(xs[:len(xs)-1], ", ") + " y " + xs[len(xs)-1]
	}
}

// pctClasificado se calcula por MONTO: 155 movimientos chicos sin partida no dicen lo mismo que
// ₡32,6M sin partida, y lo que decide si el número sirve es la plata.
//
// Sin movimientos devuelve 100 % porque aritméticamente no hay nada sin clasificar, pero ese 100 %
// NO significa que el dato sea bueno: quien avisa de un período vacío es FilaEmpresa.SinDatos, y la
// pantalla muestra eso en vez del porcentaje.
func pctClasificado(totalCRC, sinClasificarCRC string) string {
	total := aDecimal(totalCRC)
	if total.IsZero() {
		return "100.0"
	}
	sin := aDecimal(sinClasificarCRC)
	pct := total.Sub(sin).Div(total).Mul(decimal.NewFromInt(100))
	return pct.StringFixed(1)
}

// afectaEbitda: solo distorsiona el resultado lo que la naturaleza cuenta como ingreso o gasto. Un
// traslado entre empresas ya es NEUTRO y no toca el EBITDA; una regalía clasificada como GASTO sí.
func afectaEbitda(naturaleza string) bool {
	return naturaleza == "INGRESO" || naturaleza == "GASTO"
}

func esConfiable(pct string) bool {
	d, err := decimal.NewFromString(pct)
	if err != nil {
		return false
	}
	return d.GreaterThanOrEqual(decimal.NewFromFloat(umbralConfiable))
}

func todasConfiables(filas []FilaEmpresa) bool {
	for _, f := range filas {
		if !f.Confiable {
			return false
		}
	}
	return true
}

func aDecimal(s string) decimal.Decimal {
	d, err := decimal.NewFromString(s)
	if err != nil {
		return decimal.Zero
	}
	return d
}
