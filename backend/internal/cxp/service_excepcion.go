package cxp

// El arqueo de excepción (hallazgo 3.1). El porqué está en excepcion.go.

import (
	"context"

	"github.com/shopspring/decimal"
)

// ReporteDeExcepcion arma el arqueo de dos columnas.
//
// `desde`/`hasta` acotan SOLO la columna de débitos sin factura. La de facturas sin respaldo no se
// acota por rango a propósito: una factura que se declaró pagada en julio y sigue sin débito en
// octubre es exactamente la que hay que ver, y filtrarla por el mes en curso la escondería justo
// cuando más vieja —y más grave— se vuelve.
func (s *Service) ReporteDeExcepcion(ctx context.Context, empresaID, desde, hasta string) (ReporteExcepcion, error) {
	corte, err := s.repo.UltimaFechaBanco(ctx, empresaID)
	if err != nil {
		return ReporteExcepcion{}, err
	}
	rep := ReporteExcepcion{CorteBanco: corte, Desde: desde, Hasta: hasta}
	// Sin un solo movimiento cargado no hay contra qué comparar. Devolver listas vacías sería
	// decir «todo cuadra» cuando lo cierto es «no hay banco».
	if corte == "" {
		rep.TotalSinRespaldo = "0"
		rep.TotalDebitos = "0"
		rep.SinRespaldo = make([]FacturaSinRespaldo, 0)
		rep.DebitosSinFactura = make([]DebitoSinFactura, 0)
		return rep, nil
	}

	facturas, err := s.repo.FacturasSinRespaldo(ctx, empresaID, corte, "")
	if err != nil {
		return ReporteExcepcion{}, err
	}
	total := decimal.Zero
	for i := range facturas {
		if m, err := decimal.NewFromString(facturas[i].NetoCRC); err == nil {
			total = total.Add(m)
		}
		cands, err := s.repo.CandidatosPara(ctx, empresaID, facturas[i].DocumentoID, VentanaCandidatosDias)
		if err != nil {
			return ReporteExcepcion{}, err
		}
		facturas[i].Candidatos = cands
	}
	rep.SinRespaldo = facturas
	rep.TotalSinRespaldo = total.StringFixed(2)

	if rep.AunNoVencidas, err = s.repo.ContarAunNoVencidas(ctx, empresaID, corte); err != nil {
		return ReporteExcepcion{}, err
	}

	debitos, encontrados, err := s.repo.DebitosSinFactura(ctx, empresaID, desde, hasta)
	if err != nil {
		return ReporteExcepcion{}, err
	}
	rep.DebitosEncontrados = encontrados
	td := decimal.Zero
	for _, d := range debitos {
		if m, err := decimal.NewFromString(d.MontoCRC); err == nil {
			td = td.Add(m)
		}
	}
	rep.DebitosSinFactura = debitos
	rep.TotalDebitos = td.StringFixed(2)
	return rep, nil
}

// RespaldoBancarioDelLote es el mismo cálculo acotado a un lote: lo que se informa al cerrarlo.
//
// NO trae candidatos. Al cerrar, la pregunta es «¿cuánto de este corte falta ver en el banco?»,
// no «¿con cuál débito enlazo cada una?»: resolver caso por caso es el trabajo del arqueo, y
// meterlo acá volvería lento y ruidoso el momento del cierre.
func (s *Service) RespaldoBancarioDelLote(ctx context.Context, empresaID, loteID string) (RespaldoDelLote, error) {
	corte, err := s.repo.UltimaFechaBanco(ctx, empresaID)
	if err != nil {
		return RespaldoDelLote{}, err
	}
	out := RespaldoDelLote{LoteID: loteID, CorteBanco: corte, SinRespaldo: make([]FacturaSinRespaldo, 0)}
	if corte == "" {
		return out, nil
	}
	fs, err := s.repo.FacturasSinRespaldo(ctx, empresaID, corte, loteID)
	if err != nil {
		return RespaldoDelLote{}, err
	}
	out.SinRespaldo = fs
	out.Facturas = len(fs)
	if len(fs) > 0 {
		out.LoteNumero = fs[0].LoteNumero
	}
	return out, nil
}
