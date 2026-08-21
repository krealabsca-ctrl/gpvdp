package cxc

import (
	"context"
	"fmt"

	"github.com/shopspring/decimal"
)

// ConciliacionCobros es el reporte de una importación de cobros, ANTES de confirmar.
type ConciliacionCobros struct {
	Filas int `json:"filas"`
	// Aplicables: traen contrato conocido y se van a aplicar.
	Aplicables int `json:"aplicables"`
	// SinIdentificar: sin contrato, o con un contrato que no está en la cartera. Entran
	// igual, a su bandeja: la plata llegó de verdad.
	SinIdentificar int    `json:"sin_identificar"`
	Repetidos      int    `json:"repetidos"`
	Anulados       int    `json:"anulados"`
	Cuarentena     int    `json:"cuarentena"`
	Monto          string `json:"monto"`
	// ConDetalle: cuántos traen el período en el campo Concepto del origen. Es el dato que
	// permite respetar la aplicación que el sistema viejo ya había hecho.
	ConDetalle int `json:"con_detalle"`

	Muestra   []FilaCobro `json:"muestra"`
	Problemas []FilaCobro `json:"problemas"`
}

// AplicadoCobros es el resultado de confirmar.
type AplicadoCobros struct {
	Registrados    int    `json:"registrados"`
	Repetidos      int    `json:"repetidos"`
	SinIdentificar int    `json:"sin_identificar"`
	Aplicado       string `json:"aplicado"`
	SaldoAFavor    string `json:"saldo_a_favor"`
}

// PrevisualizarCobros lee el archivo de pagos y dice qué pasaría, sin tocar la cartera.
func (s *Service) PrevisualizarCobros(ctx context.Context, empresaID string, archivo []byte, nombre, usuarioID string) (string, ConciliacionCobros, error) {
	filas, err := s.leerCobros(ctx, empresaID, archivo)
	if err != nil {
		return "", ConciliacionCobros{}, err
	}
	rep, err := s.conciliarCobros(ctx, empresaID, filas)
	if err != nil {
		return "", ConciliacionCobros{}, err
	}
	// El reporte de cobros se guarda con la misma cabecera que el de cartera: la pregunta
	// «¿qué entró el 4 de agosto?» se hace semanas después.
	id, err := s.repo.CrearImportacion(ctx, empresaID, "COBROS", nombre, usuarioID, Conciliacion{
		Filas: rep.Filas, Nuevos: rep.Aplicables, Duplicados: rep.Repetidos, Cuarentena: rep.Cuarentena,
	})
	if err != nil {
		return "", ConciliacionCobros{}, err
	}
	return id, rep, nil
}

// ConfirmarCobros registra los cobros del archivo. Cada cobro es su propia transacción a
// propósito: con miles de pagos, que una fila mala tumbe el lote entero sería peor que
// registrar lo bueno y reportar lo que falló. La idempotencia hace que reintentar sea seguro.
func (s *Service) ConfirmarCobros(ctx context.Context, empresaID, importacionID string, archivo []byte, usuarioID string) (ConciliacionCobros, AplicadoCobros, []string, error) {
	filas, err := s.leerCobros(ctx, empresaID, archivo)
	if err != nil {
		return ConciliacionCobros{}, AplicadoCobros{}, nil, err
	}
	rep, err := s.conciliarCobros(ctx, empresaID, filas)
	if err != nil {
		return ConciliacionCobros{}, AplicadoCobros{}, nil, err
	}

	var out AplicadoCobros
	aplicado, favor := decimal.Zero, decimal.Zero
	fallas := []string{}
	for _, f := range filas {
		// Los anulados del origen no entran como cobros vivos, y los que no tienen monto
		// legible tampoco: registrar un monto inventado es peor que no registrarlo.
		if f.Anulado() || f.Monto.Sign() <= 0 {
			continue
		}
		in := CobroInput{
			Contrato: f.Contrato, Consecutivo: f.Consecutivo,
			FechaPago: f.FechaPago, FechaBancaria: f.FechaBanco, FechaRegistro: f.FechaCreado,
			Monto: f.Monto, FormaPago: f.FormaPago, Asociacion: f.Asociacion,
			Referencia: f.Referencia, Concepto: f.Concepto, Origen: "ARCHIVO",
			// La llave del archivo: el consecutivo del origen. Reimportar el mismo archivo
			// no duplica ni un cobro.
			IdempotencyKey: llaveDeArchivo(f),
		}
		res, err := s.repo.RegistrarCobro(ctx, empresaID, in, usuarioID)
		if err != nil {
			fallas = append(fallas, fmt.Sprintf("línea %d (consecutivo %s): %v", f.Linea, f.Consecutivo, err))
			continue
		}
		switch {
		case res.Repetido:
			out.Repetidos++
		case res.Estado == CobroSinIdentificar:
			out.SinIdentificar++
			out.Registrados++
		default:
			out.Registrados++
		}
		for _, a := range res.Aplicaciones {
			aplicado = aplicado.Add(a.Monto)
		}
		if v, err := decimal.NewFromString(res.SaldoAFavor); err == nil {
			favor = favor.Add(v)
		}
	}
	out.Aplicado, out.SaldoAFavor = aplicado.String(), favor.String()

	if importacionID != "" {
		if err := s.repo.ConfirmarImportacion(ctx, empresaID, importacionID); err != nil {
			return rep, out, fallas, err
		}
	}
	s.auditar(ctx, empresaID, "IMPORTAR_COBROS_CXC", usuarioID, map[string]any{
		"filas": rep.Filas, "registrados": out.Registrados, "repetidos": out.Repetidos,
		"sin_identificar": out.SinIdentificar, "aplicado": out.Aplicado, "fallas": len(fallas),
	})
	return rep, out, fallas, nil
}

// llaveDeArchivo arma la llave de idempotencia de una fila del archivo. Con el consecutivo
// del origen basta: es único por recibo en su sistema.
func llaveDeArchivo(f FilaCobro) string {
	if f.Consecutivo == "" {
		return ""
	}
	return "archivo:" + f.Consecutivo
}

func (s *Service) leerCobros(ctx context.Context, empresaID string, archivo []byte) ([]FilaCobro, error) {
	g, err := CargarGrid(archivo)
	if err != nil {
		return nil, err
	}
	p, err := s.repo.Parametros(ctx, empresaID)
	if err != nil {
		return nil, err
	}
	reglas := ReglasCobros{}
	if v, ok := p["COBRO_MAXIMO_RAZONABLE"]; ok && v != "" {
		if m, err := decimal.NewFromString(v); err == nil {
			reglas.CobroMaximo = m
		}
	}
	return LeerCobros(g, reglas)
}

func (s *Service) conciliarCobros(ctx context.Context, empresaID string, filas []FilaCobro) (ConciliacionCobros, error) {
	rep := ConciliacionCobros{Filas: len(filas), Muestra: []FilaCobro{}, Problemas: []FilaCobro{}}
	// Qué contratos del archivo existen en la cartera: eso decide si un cobro se aplica o
	// se va a la bandeja de sin identificar.
	numeros := make([]string, 0, len(filas))
	for _, f := range filas {
		if f.Contrato != "" {
			numeros = append(numeros, f.Contrato)
		}
	}
	existentes, err := s.repo.NumerosExistentes(ctx, empresaID, numeros)
	if err != nil {
		return ConciliacionCobros{}, err
	}
	monto := decimal.Zero
	for _, f := range filas {
		if f.Anulado() {
			rep.Anulados++
			continue
		}
		monto = monto.Add(f.Monto)
		if f.Contrato != "" && existentes[f.Contrato] {
			rep.Aplicables++
		} else {
			rep.SinIdentificar++
		}
		if len(f.Aplicaciones) > 0 {
			rep.ConDetalle++
		}
		if f.EnCuarentena() {
			rep.Cuarentena++
			if len(rep.Problemas) < 500 {
				rep.Problemas = append(rep.Problemas, f)
			}
		}
		if len(rep.Muestra) < 10 {
			rep.Muestra = append(rep.Muestra, f)
		}
	}
	rep.Monto = monto.String()
	return rep, nil
}

// RegistrarCobro es la vía de la API y de la caja: un cobro a la vez, idempotente.
func (s *Service) RegistrarCobro(ctx context.Context, empresaID string, in CobroInput, usuarioID string) (CobroRegistrado, error) {
	res, err := s.repo.RegistrarCobro(ctx, empresaID, in, usuarioID)
	if err != nil {
		return CobroRegistrado{}, err
	}
	if !res.Repetido {
		s.auditar(ctx, empresaID, "REGISTRAR_COBRO_CXC", usuarioID, map[string]any{
			"cobro": res.ID, "contrato": in.Contrato, "monto": in.Monto.String(),
			"aplicaciones": len(res.Aplicaciones), "estado": res.Estado,
		})
	}
	return res, nil
}

// ReversarCobro deshace un cobro que no entró (cheque devuelto, débito rechazado).
func (s *Service) ReversarCobro(ctx context.Context, empresaID, cobroID, motivo, usuarioID string) error {
	if err := s.repo.ReversarCobro(ctx, empresaID, cobroID, motivo, usuarioID); err != nil {
		return err
	}
	s.auditar(ctx, empresaID, "REVERSAR_COBRO_CXC", usuarioID, map[string]any{
		"cobro": cobroID, "motivo": motivo,
	})
	return nil
}

// IdentificarCobro le asigna contrato a un depósito que entró sin dueño y lo aplica.
func (s *Service) IdentificarCobro(ctx context.Context, empresaID, cobroID, contrato, usuarioID string) (CobroRegistrado, error) {
	res, err := s.repo.IdentificarCobro(ctx, empresaID, cobroID, contrato, usuarioID)
	if err != nil {
		return CobroRegistrado{}, err
	}
	s.auditar(ctx, empresaID, "IDENTIFICAR_COBRO_CXC", usuarioID, map[string]any{
		"cobro": cobroID, "contrato": contrato, "aplicaciones": len(res.Aplicaciones),
	})
	return res, nil
}

// ListarCobros devuelve los cobros filtrados con el resumen de lo filtrado.
func (s *Service) ListarCobros(ctx context.Context, empresaID string, f FiltrosCobros) (ListaCobros, error) {
	return s.repo.ListarCobros(ctx, empresaID, f)
}

// PanoramaAsociaciones es el estado del canal de asociaciones en un período.
func (s *Service) PanoramaAsociaciones(ctx context.Context, empresaID, periodo string) (PanoramaAsociaciones, error) {
	return s.repo.PanoramaAsociaciones(ctx, empresaID, periodo, s.toleranciaPlanilla(ctx, empresaID))
}
