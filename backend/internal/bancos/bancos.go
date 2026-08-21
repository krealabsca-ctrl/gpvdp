// Package bancos implementa el importador de estados de cuenta y los adaptadores por banco.
// El parseo opera sobre una Grid ([][]string) para ser testeable sin archivos reales;
// la carga desde Excel vive en excel.go. Ver docs/GPVDP_Formatos_Bancos_v1.0.md.
package bancos

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// Banco identifica el banco de origen de un estado de cuenta.
type Banco string

const (
	BancoPromerica  Banco = "Promerica"
	BancoBN         Banco = "BN"
	BancoBAC        Banco = "BAC"
	BancoBCR        Banco = "BCR"
	BancoBP         Banco = "Banco Popular"
	BancoDavivienda Banco = "Davivienda"
)

// ErrNoReconocido indica que ningún adaptador reconoce el archivo.
var ErrNoReconocido = errors.New("bancos: formato de archivo no reconocido")

// MovimientoParsed es una línea normalizada del estado de cuenta, antes de persistir.
// El natural_key se calcula en la capa de servicio porque necesita cuenta_bancaria_id.
type MovimientoParsed struct {
	Fecha            time.Time
	Documento        string
	Descripcion      string
	Debito           decimal.Decimal
	Credito          decimal.Decimal
	IndiceOcurrencia int
}

// ParseResult es el resultado de parsear un archivo con un adaptador.
type ParseResult struct {
	Banco         Banco
	IBAN          string // "" si el archivo no trae IBAN (Promerica/BN)
	MonedaArchivo string // "CRC"/"USD"/"" — verificación cruzada; la moneda real viene de la cuenta
	Movimientos   []MovimientoParsed
}

// Adapter es la estrategia de parseo de un banco (patrón strategy).
type Adapter interface {
	Banco() Banco
	Detecta(g Grid) bool
	Parsea(g Grid) (ParseResult, error)
}

// Adapters devuelve todos los adaptadores registrados.
func Adapters() []Adapter { return []Adapter{promerica, bn, bac, bcr, bp, davivienda} }

// Detectar elige el primer adaptador que reconoce el archivo.
func Detectar(g Grid) (Adapter, error) {
	for _, a := range Adapters() {
		if a.Detecta(g) {
			return a, nil
		}
	}
	return nil, ErrNoReconocido
}

// bankSpec describe un formato de banco de forma declarativa.
type bankSpec struct {
	banco                                            Banco
	sigToken                                         string // texto único que debe aparecer (normalizado); "" = ninguno
	colFecha, colDoc, colDesc, colDebito, colCredito int
	fecha                                            func(string) (time.Time, error)
	isHeader                                         func(cells []string) bool
}

func (s *bankSpec) Banco() Banco        { return s.banco }
func (s *bankSpec) Detecta(g Grid) bool { _, ok := s.detect(g); return ok }

func (s *bankSpec) detect(g Grid) (int, bool) {
	if s.sigToken != "" && !gridContains(g, s.sigToken) {
		return 0, false
	}
	for r := 0; r < len(g) && r < 25; r++ {
		if s.isHeader(g[r]) {
			return r, true
		}
	}
	return 0, false
}

// Parsea localiza el encabezado y normaliza cada fila de movimiento al canónico.
// Toda fila cuya celda de fecha no parsee (saldo inicial, totales, pie, vacío) se descarta.
func (s *bankSpec) Parsea(g Grid) (ParseResult, error) {
	headerRow, ok := s.detect(g)
	if !ok {
		return ParseResult{}, ErrNoReconocido
	}
	var movs []MovimientoParsed
	// Descartar filas sin fecha es CORRECTO (saldo inicial, subtotales, pie). Lo que no puede
	// pasar en silencio es descartarlas TODAS: se cuentan las que traían algo escrito en la
	// celda de fecha y aun así no se entendieron, para poder distinguir «el banco no mandó
	// movimientos» de «el banco cambió el formato». Ver FechasIlegiblesError.
	ilegibles := 0
	muestraIlegible := ""
	for r := headerRow + 1; r < len(g); r++ {
		cells := g[r]
		crudo := cell(cells, s.colFecha)
		fecha, err := s.fecha(crudo)
		if err != nil {
			if strings.TrimSpace(crudo) != "" {
				ilegibles++
				if muestraIlegible == "" {
					muestraIlegible = strings.TrimSpace(crudo)
				}
			}
			continue
		}
		deb, err := parseMonto(cell(cells, s.colDebito))
		if err != nil {
			return ParseResult{}, fmt.Errorf("bancos: fila %d débito: %w", r+1, err)
		}
		cred, err := parseMonto(cell(cells, s.colCredito))
		if err != nil {
			return ParseResult{}, fmt.Errorf("bancos: fila %d crédito: %w", r+1, err)
		}
		movs = append(movs, MovimientoParsed{
			Fecha:       fecha,
			Documento:   strings.TrimSpace(cell(cells, s.colDoc)),
			Descripcion: strings.TrimSpace(cell(cells, s.colDesc)),
			Debito:      deb,
			Credito:     cred,
		})
	}
	if len(movs) == 0 && ilegibles > 0 {
		return ParseResult{}, &FechasIlegiblesError{Banco: s.banco, Filas: ilegibles, Muestra: muestraIlegible}
	}
	asignarIndiceOcurrencia(movs)
	return ParseResult{
		Banco:         s.banco,
		IBAN:          extraerIBAN(g),
		MonedaArchivo: extraerMoneda(g),
		Movimientos:   movs,
	}, nil
}

// asignarIndiceOcurrencia numera 1,2,… los duplicados legítimos dentro del archivo
// por la tupla (fecha, débito, crédito, documento) — caso BAC (RN-07/09).
func asignarIndiceOcurrencia(movs []MovimientoParsed) {
	seen := make(map[string]int)
	for i := range movs {
		k := movs[i].Fecha.Format("2006-01-02") + "|" +
			movs[i].Debito.String() + "|" + movs[i].Credito.String() + "|" + movs[i].Documento
		seen[k]++
		movs[i].IndiceOcurrencia = seen[k]
	}
}
