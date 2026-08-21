package bancos

import (
	"context"

	"github.com/shopspring/decimal"
)

// Árbol del cuadre: Concepto → Clasificación, cada uno con Débito y Crédito y su
// conteo de movimientos. Incluye TODO (traslados incluidos: así se ve que cuadran).
// Totales: crédito = Ingresos, débito = Gastos.
type CuadreClasifNodo struct {
	ClasificacionID string `json:"clasificacion_id"`
	Clasificacion   string `json:"clasificacion"`
	Debito          string `json:"debito"`
	Credito         string `json:"credito"`
	Movs            int    `json:"movs"`
}

type CuadreConceptoNodo struct {
	ConceptoID      string             `json:"concepto_id"`
	Concepto        string             `json:"concepto"`
	Debito          string             `json:"debito"`
	Credito         string             `json:"credito"`
	Movs            int                `json:"movs"`
	Clasificaciones []CuadreClasifNodo `json:"clasificaciones"`
}

type CuadreArbolData struct {
	Periodo      string               `json:"periodo"`
	TotalDebito  string               `json:"total_debito"`
	TotalCredito string               `json:"total_credito"`
	Movs         int                  `json:"movs"`
	Conceptos    []CuadreConceptoNodo `json:"conceptos"`
}

// CuadreArbol arma el árbol Concepto → Clasificación del período, con Débito y
// Crédito por nodo (montos en CRC, mismo filtro que Cuadre; incluye traslados).
func (s *Service) CuadreArbol(ctx context.Context, empresaID, periodo string) (CuadreArbolData, error) {
	filas, err := s.repo.CuadreArbol(ctx, empresaID, periodo)
	if err != nil {
		return CuadreArbolData{}, err
	}
	b := newCuadreBuilder()
	for _, f := range filas {
		b.add(f.ConceptoID, f.Concepto, f.ClasificacionID, f.Clasificacion,
			f.DebitoSum, f.CreditoSum, f.DebitoCnt+f.CreditoCnt)
	}
	return b.build(periodo), nil
}

// --- Constructor con orden preservado ---

type clasifAcc struct {
	id, nombre      string
	debito, credito decimal.Decimal
	movs            int
}
type conceptoAcc struct {
	id, nombre      string
	debito, credito decimal.Decimal
	movs            int
	clIdx           map[string]int
	cls             []*clasifAcc
}
type cuadreBuilder struct {
	totalDeb, totalCred decimal.Decimal
	movs                int
	idx                 map[string]int
	concs               []*conceptoAcc
}

func newCuadreBuilder() *cuadreBuilder { return &cuadreBuilder{idx: map[string]int{}} }

func (b *cuadreBuilder) add(cid, cnom, clid, clnom string, deb, cred decimal.Decimal, movs int) {
	b.totalDeb = b.totalDeb.Add(deb)
	b.totalCred = b.totalCred.Add(cred)
	b.movs += movs

	ci, ok := b.idx[cid]
	if !ok {
		ci = len(b.concs)
		b.idx[cid] = ci
		b.concs = append(b.concs, &conceptoAcc{id: cid, nombre: cnom, clIdx: map[string]int{}})
	}
	c := b.concs[ci]
	c.debito = c.debito.Add(deb)
	c.credito = c.credito.Add(cred)
	c.movs += movs

	cli, ok := c.clIdx[clid]
	if !ok {
		cli = len(c.cls)
		c.clIdx[clid] = cli
		c.cls = append(c.cls, &clasifAcc{id: clid, nombre: clnom})
	}
	cl := c.cls[cli]
	cl.debito = cl.debito.Add(deb)
	cl.credito = cl.credito.Add(cred)
	cl.movs += movs
}

func (b *cuadreBuilder) build(periodo string) CuadreArbolData {
	out := CuadreArbolData{
		Periodo:      periodo,
		TotalDebito:  b.totalDeb.String(),
		TotalCredito: b.totalCred.String(),
		Movs:         b.movs,
		Conceptos:    make([]CuadreConceptoNodo, 0, len(b.concs)),
	}
	for _, c := range b.concs {
		nodo := CuadreConceptoNodo{
			ConceptoID: c.id, Concepto: c.nombre,
			Debito: c.debito.String(), Credito: c.credito.String(), Movs: c.movs,
			Clasificaciones: make([]CuadreClasifNodo, 0, len(c.cls)),
		}
		for _, cl := range c.cls {
			nodo.Clasificaciones = append(nodo.Clasificaciones, CuadreClasifNodo{
				ClasificacionID: cl.id, Clasificacion: cl.nombre,
				Debito: cl.debito.String(), Credito: cl.credito.String(), Movs: cl.movs,
			})
		}
		out.Conceptos = append(out.Conceptos, nodo)
	}
	return out
}
