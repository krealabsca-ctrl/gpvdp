package cxp

import (
	"context"
	"errors"
	"strings"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// PreviewImportacion parsea el Excel y marca cada fila (nueva/duplicada por clave) y si su
// proveedor (por cédula) ya existe. No crea nada.
func (s *Service) PreviewImportacion(ctx context.Context, empresaID string, data []byte) (PreviewImportacion, error) {
	filas, err := parsearFacturas(data)
	if err != nil {
		return PreviewImportacion{}, err
	}
	existentes, err := s.repo.ClavesExistentes(ctx, empresaID, clavesUnicas(filas))
	if err != nil {
		return PreviewImportacion{}, err
	}

	cedulaExiste := map[string]bool{}
	contadas := map[string]bool{}
	var res ResumenImportacion
	for i := range filas {
		if existentes[filas[i].Clave] {
			filas[i].Estado = ImpDuplicado
			res.Duplicadas++
		} else {
			filas[i].Estado = ImpNuevo
			res.Nuevas++
		}
		ced := filas[i].Cedula
		existe, visto := cedulaExiste[ced]
		if !visto {
			_, found, err := s.repo.ProveedorIDPorIdentificacion(ctx, empresaID, ced)
			if err != nil {
				return PreviewImportacion{}, err
			}
			existe = found
			cedulaExiste[ced] = found
		}
		if !existe {
			filas[i].ProveedorNuevo = true
			if ced != "" && !contadas[ced] {
				res.ProveedoresNuevos++
				contadas[ced] = true
			}
		}
	}
	res.Leidas = len(filas)
	return PreviewImportacion{Resumen: res, Filas: filas}, nil
}

// ConfirmarImportacion crea los documentos nuevos (omite duplicados por clave) y da de alta
// los proveedores que no existan (por cédula). Best-effort por fila: acumula errores.
func (s *Service) ConfirmarImportacion(ctx context.Context, empresaID string, data []byte, usuarioID string) (ResultadoImportacion, error) {
	filas, err := parsearFacturas(data)
	if err != nil {
		return ResultadoImportacion{}, err
	}
	existentes, err := s.repo.ClavesExistentes(ctx, empresaID, clavesUnicas(filas))
	if err != nil {
		return ResultadoImportacion{}, err
	}

	cache := map[string]string{}                     // cédula -> proveedorID
	vistas := map[string]bool{}                      // claves ya procesadas en este archivo (dup intra-archivo)
	out := ResultadoImportacion{Errores: []string{}} // nunca nil → JSON [] (no null)
	for _, fila := range filas {
		if existentes[fila.Clave] || vistas[fila.Clave] {
			out.OmitidosDuplicados++
			continue
		}
		vistas[fila.Clave] = true

		provID, err := s.resolverProveedor(ctx, empresaID, fila, cache, &out, usuarioID)
		if err != nil {
			out.Errores = append(out.Errores, fila.Clave+": "+err.Error())
			continue
		}
		in, err := filaAInput(fila, provID)
		if err != nil {
			out.Errores = append(out.Errores, fila.Clave+": "+err.Error())
			continue
		}
		if _, err := s.CrearDocumento(ctx, empresaID, in, usuarioID); err != nil {
			if errors.Is(err, ErrDocumentoDuplicado) {
				out.OmitidosDuplicados++
			} else {
				out.Errores = append(out.Errores, fila.Clave+": "+err.Error())
			}
			continue
		}
		out.Creados++
	}
	return out, nil
}

// resolverProveedor devuelve el id del proveedor (por cédula), creándolo si no existe.
// En ambos casos aprende las condiciones de pago de la factura (Condición + plazo del Excel):
// al crear las fija, y a un proveedor existente que siga en Contado/0 se las completa.
func (s *Service) resolverProveedor(ctx context.Context, empresaID string, fila FilaImportada, cache map[string]string, out *ResultadoImportacion, usuarioID string) (string, error) {
	cond, plazo := condicionDeFila(fila)
	ced := fila.Cedula
	if ced != "" {
		if id, ok := cache[ced]; ok {
			return id, nil
		}
		id, found, err := s.repo.ProveedorIDPorIdentificacion(ctx, empresaID, ced)
		if err != nil {
			return "", err
		}
		if found {
			if cond == "CREDITO" {
				if e := s.repo.AprenderCondicionPago(ctx, empresaID, id, cond, plazo); e != nil {
					s.log.Warn("cxp: no se pudo aprender la condición de pago", zap.Error(e))
				}
			}
			cache[ced] = id
			return id, nil
		}
	}
	nombre := strings.TrimSpace(fila.Proveedor)
	if nombre == "" {
		nombre = "(sin nombre)"
	}
	p, err := s.Crear(ctx, empresaID, ProveedorInput{
		Nombre:             nombre,
		TipoIdentificacion: tipoIdentificacion(ced),
		Identificacion:     ced,
		CondicionPago:      cond,
		PlazoCreditoDias:   plazo,
	}, usuarioID)
	if err != nil {
		// Carrera: si otro proceso ya lo creó, recuperarlo.
		if errors.Is(err, ErrProveedorDuplicado) && ced != "" {
			if id, found, e := s.repo.ProveedorIDPorIdentificacion(ctx, empresaID, ced); e == nil && found {
				cache[ced] = id
				return id, nil
			}
		}
		return "", err
	}
	if ced != "" {
		cache[ced] = p.ID
	}
	out.ProveedoresCreados++
	return p.ID, nil
}

// filaAInput convierte una fila del Excel en DocumentoInput. USD requiere TC (no viene en el
// Excel): esas filas se rechazan para no calcular un total_crc incorrecto.
func filaAInput(fila FilaImportada, provID string) (DocumentoInput, error) {
	if fila.Moneda == "USD" {
		return DocumentoInput{}, errors.New("factura en USD: cargala manualmente con su tipo de cambio")
	}
	sub, err := decOrZero(fila.Subtotal)
	if err != nil {
		return DocumentoInput{}, errors.New("subtotal inválido")
	}
	iva, err := decOrZero(fila.IVA)
	if err != nil {
		return DocumentoInput{}, errors.New("impuestos inválidos")
	}
	total, err := decimal.NewFromString(strings.TrimSpace(fila.Total))
	if err != nil {
		return DocumentoInput{}, errors.New("total inválido")
	}
	return DocumentoInput{
		ProveedorID:  provID,
		Clave:        fila.Clave,
		Consecutivo:  fila.Consecutivo,
		FechaEmision: fila.FechaEmision,
		Moneda:       "CRC",
		Subtotal:     sub,
		IVA:          iva,
		Retencion:    decimal.Zero,
		Total:        total,
		TC:           decimal.Zero,
		Descripcion:  descripcionImport(fila),
		Vencimiento:  fechaISO(fila.Vencimiento),
	}, nil
}

// descripcionImport arma la descripción (la fecha de vencimiento ya va en su propio campo).
func descripcionImport(f FilaImportada) string {
	parts := []string{"Importado de facturación"}
	if f.Condicion != "" {
		parts = append(parts, "Condición: "+f.Condicion)
	}
	return strings.Join(parts, " · ")
}

func clavesUnicas(filas []FilaImportada) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(filas))
	for _, f := range filas {
		if f.Clave != "" && !seen[f.Clave] {
			seen[f.Clave] = true
			out = append(out, f.Clave)
		}
	}
	return out
}

// tipoIdentificacion infiere el tipo por longitud de la cédula CR (física 9 / jurídica 10).
func tipoIdentificacion(ced string) string {
	switch len(strings.TrimSpace(ced)) {
	case 9:
		return "FISICA"
	case 10:
		return "JURIDICA"
	default:
		return ""
	}
}
