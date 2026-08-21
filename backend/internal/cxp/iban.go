package cxp

// Cuentas IBAN de los proveedores: carga masiva y validación.
//
// Por qué existe esto: la macro de pago que va al banco lleva el IBAN en el primer campo. Sin él,
// el banco no tiene a dónde mandar la plata y rechaza la línea. Al medirlo, 648 de 649 proveedores
// no tenían IBAN, así que la macro salía inservible y nada avisaba. Completarlos uno por uno desde
// la ficha no es viable, de ahí la carga por Excel.

import (
	"context"
	"sort"
	"strings"
	"unicode"
)

// Largo del IBAN de Costa Rica: "CR" + 2 dígitos de control + 18 del número de cuenta.
const largoIBANCR = 22

// Resultados posibles de validar una fila de la carga.
const (
	IBANOK           = "OK"            // válido y se va a guardar
	IBANIgual        = "SIN_CAMBIO"    // el proveedor ya tenía exactamente ese IBAN
	IBANInvalido     = "INVALIDO"      // no tiene forma de IBAN de Costa Rica
	IBANSinProveedor = "NO_ENCONTRADO" // la cédula no corresponde a ningún proveedor de la empresa
	IBANDuplicado    = "DUPLICADO"     // el archivo trae la misma cédula dos veces
)

// FilaIBAN es una línea del archivo tal como se leyó, con su veredicto.
type FilaIBAN struct {
	Fila           int    `json:"fila"`
	Identificacion string `json:"identificacion"`
	Nombre         string `json:"nombre"`
	IBAN           string `json:"iban"`
	Estado         string `json:"estado"`
	Detalle        string `json:"detalle"`
	// ProveedorID queda vacío si no se encontró.
	ProveedorID string `json:"proveedor_id"`
	// IBANAnterior permite ver qué se está reemplazando: sobrescribir una cuenta sin mostrar la
	// anterior es como cambiarla a ciegas.
	IBANAnterior string `json:"iban_anterior"`
}

// ResumenIBAN es lo que la pantalla muestra antes de confirmar.
type ResumenIBAN struct {
	Filas      []FilaIBAN `json:"filas"`
	ACargar    int        `json:"a_cargar"`
	SinCambio  int        `json:"sin_cambio"`
	Invalidos  int        `json:"invalidos"`
	NoHallados int        `json:"no_hallados"`
	Duplicados int        `json:"duplicados"`
}

// NormalizarIBAN deja el IBAN como lo espera el banco: sin espacios ni guiones, en mayúsculas.
// La gente lo copia del estado de cuenta con espacios cada 4 caracteres, y así viene del Excel.
func NormalizarIBAN(s string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// ValidarIBANCR comprueba la forma del IBAN de Costa Rica: CR + 20 dígitos.
//
// Solo valida la FORMA, no el dígito de control: un IBAN bien formado puede seguir siendo de otra
// cuenta, y de eso avisa el banco. La forma alcanza para atajar el error frecuente —pegar el
// número de cuenta sin el prefijo, o dejar una celda a medias— antes de generar la macro.
func ValidarIBANCR(iban string) (string, bool) {
	n := NormalizarIBAN(iban)
	if len(n) != largoIBANCR {
		return n, false
	}
	if !strings.HasPrefix(n, "CR") {
		return n, false
	}
	for _, r := range n[2:] {
		if !unicode.IsDigit(r) {
			return n, false
		}
	}
	return n, true
}

// PrevisualizarIBAN lee las filas de un archivo ya parseado y dice qué va a pasar con cada una,
// SIN escribir nada. Es el mismo criterio del resto de los importadores del sistema: primero se
// ve, después se confirma.
func (s *Service) PrevisualizarIBAN(ctx context.Context, empresaID string, filas []FilaIBAN) (ResumenIBAN, error) {
	porCedula, err := s.repo.ProveedoresPorIdentificacion(ctx, empresaID)
	if err != nil {
		return ResumenIBAN{}, err
	}
	vistas := make(map[string]int, len(filas))
	out := ResumenIBAN{Filas: make([]FilaIBAN, 0, len(filas))}
	for _, f := range filas {
		f.Identificacion = strings.TrimSpace(f.Identificacion)
		iban, ok := ValidarIBANCR(f.IBAN)
		f.IBAN = iban

		switch {
		case f.Identificacion == "":
			f.Estado, f.Detalle = IBANInvalido, "falta la identificación del proveedor"
		case !ok:
			f.Estado = IBANInvalido
			if iban == "" {
				f.Detalle = "la celda del IBAN está vacía"
			} else {
				f.Detalle = "no tiene forma de IBAN de Costa Rica (CR + 20 dígitos); vino " + iban
			}
		default:
			k := clave(f.Identificacion)
			if antes, repetida := vistas[k]; repetida {
				f.Estado = IBANDuplicado
				f.Detalle = "esta identificación ya venía en la fila " + itoa(antes)
				break
			}
			prov, existe := porCedula[k]
			if !existe {
				f.Estado = IBANSinProveedor
				f.Detalle = "ningún proveedor de esta empresa tiene la identificación " + f.Identificacion
				break
			}
			f.ProveedorID, f.Nombre, f.IBANAnterior = prov.ID, prov.Nombre, prov.IBAN
			if NormalizarIBAN(prov.IBAN) == iban {
				f.Estado, f.Detalle = IBANIgual, "ya tenía este IBAN"
			} else {
				f.Estado = IBANOK
				if prov.IBAN != "" {
					f.Detalle = "reemplaza el IBAN anterior"
				}
			}
			vistas[k] = f.Fila
		}

		switch f.Estado {
		case IBANOK:
			out.ACargar++
		case IBANIgual:
			out.SinCambio++
		case IBANInvalido:
			out.Invalidos++
		case IBANSinProveedor:
			out.NoHallados++
		case IBANDuplicado:
			out.Duplicados++
		}
		out.Filas = append(out.Filas, f)
	}
	return out, nil
}

// CargarIBAN guarda las filas válidas. Devuelve cuántos proveedores quedaron actualizados.
//
// Solo escribe las que la previsualización marcó como OK: una carga que "arregla" filas inválidas
// por su cuenta terminaría inventando cuentas bancarias, que es lo último que debe pasar acá.
func (s *Service) CargarIBAN(ctx context.Context, empresaID string, filas []FilaIBAN, usuarioID string) (int, error) {
	resumen, err := s.PrevisualizarIBAN(ctx, empresaID, filas)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, f := range resumen.Filas {
		if f.Estado != IBANOK || f.ProveedorID == "" {
			continue
		}
		if err := s.repo.ActualizarIBANProveedor(ctx, empresaID, f.ProveedorID, f.IBAN); err != nil {
			return n, err
		}
		// Cambiar una cuenta bancaria es sensible: queda con quién, cuándo y qué había antes.
		s.auditarEntidad(ctx, empresaID, "proveedor", f.ProveedorID, "CARGAR_IBAN", usuarioID)
		n++
	}
	return n, nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var d []byte
	for n > 0 {
		d = append([]byte{byte('0' + n%10)}, d...)
		n /= 10
	}
	return string(d)
}

// FaltanIBAN devuelve los proveedores de un archivo de pago que no tienen cuenta destino.
//
// Guardarraíl: el banco rechaza la línea sin IBAN, y hasta ahora la macro se generaba igual sin
// decir nada. Con 648 proveedores sin cuenta, eso significaba bajar un archivo inservible y
// enterarse en la ventanilla del banco. La lista sale por nombre para poder ir a completarlos.
func FaltanIBAN(rows []PagoRow) []PagoRow {
	var sin []PagoRow
	for _, r := range rows {
		if _, ok := ValidarIBANCR(r.IBAN); !ok {
			sin = append(sin, r)
		}
	}
	return sin
}

// ProveedoresSinIBAN son los proveedores activos que todavía no se pueden pagar por transferencia.
func (s *Service) ProveedoresSinIBAN(ctx context.Context, empresaID string) ([]ProveedorIBAN, error) {
	todos, err := s.repo.ProveedoresPorIdentificacion(ctx, empresaID)
	if err != nil {
		return nil, err
	}
	sin := make([]ProveedorIBAN, 0)
	for _, p := range todos {
		if _, ok := ValidarIBANCR(p.IBAN); !ok {
			sin = append(sin, p)
		}
	}
	sort.Slice(sin, func(i, j int) bool { return sin[i].Nombre < sin[j].Nombre })
	return sin, nil
}
