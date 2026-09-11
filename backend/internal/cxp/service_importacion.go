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
	filas, traza, err := parsearEntrega(data)
	if err != nil {
		return PreviewImportacion{}, err
	}
	existentes, err := s.repo.ClavesExistentes(ctx, empresaID, clavesUnicas(filas))
	if err != nil {
		return PreviewImportacion{}, err
	}

	cedulaExiste := map[string]bool{}
	contadas := map[string]bool{}
	// La traza de lectura viaja al preview: la pantalla tiene que poder decir POR QUÉ leyó menos
	// facturas de las que el archivo parece tener.
	res := traza
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
	filas, _, err := parsearEntrega(data)
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

// filaAInput convierte una fila del Excel en DocumentoInput.
//
// ── LAS DOS COSAS QUE SE ARREGLARON ACÁ (2026-09-09) ────────────────────────
//
//  1. USD ya no se rechaza. Antes decía «cargala manualmente con su tipo de cambio» porque se
//     suponía que el TC no venía en el archivo — y sí viene: la columna «Tipo Cambio». Se usa el TC
//     DE LA FACTURA, que es el que manda para lo que se le debe al proveedor (el congelado del mes
//     es de Bancos). Sin TC no se inventa uno: la fila se rechaza diciendo qué falta.
//
//  2. La fecha de emisión se exige. Antes viajaba cruda al `::date` de Postgres, que está en MDY:
//     las de día 13 al 31 reventaban y las de día 1 al 12 entraban con el mes y el día invertidos,
//     en silencio. Ahora viene normalizada del parser, y si no se entendió se rechaza la fila —una
//     factura sin fecha de emisión no tiene vencimiento ni aging, así que no sirve igual—.
func filaAInput(fila FilaImportada, provID string) (DocumentoInput, error) {
	if fila.FechaEmision == "" {
		return DocumentoInput{}, errors.New("fecha de emisión ilegible o vacía (se esperan dd/mm/aaaa o aaaa-mm-dd)")
	}
	moneda := fila.Moneda
	if moneda == "" {
		moneda = "CRC"
	}
	tc := decimal.Zero
	switch moneda {
	case "CRC":
		// El TC de una factura en colones no se guarda: sería 1 o basura, y en los dos casos
		// confunde al leer el expediente.
	case "USD":
		v, err := decimal.NewFromString(strings.TrimSpace(fila.TC))
		if err != nil || v.LessThanOrEqual(decimal.Zero) {
			return DocumentoInput{}, errors.New("factura en USD sin tipo de cambio en el archivo (columna «Tipo Cambio»)")
		}
		tc = v
	default:
		// La base solo acepta CRC y USD (CHECK de `documento_cxp.moneda`). Se dice cuál es la
		// moneda para que se entienda por qué no entró, en vez de un 500 desde el CHECK.
		return DocumentoInput{}, errors.New("moneda " + moneda + " no soportada: el sistema maneja CRC y USD")
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
	in := DocumentoInput{
		ProveedorID:  provID,
		Clave:        fila.Clave,
		Consecutivo:  fila.Consecutivo,
		FechaEmision: fila.FechaEmision,
		Moneda:       moneda,
		Subtotal:     sub,
		IVA:          iva,
		Retencion:    decimal.Zero,
		Total:        total,
		TC:           tc,
		Descripcion:  descripcionImport(fila),
		// Ya viene normalizada del parser: pasarla por `fechaISO` otra vez sería inofensivo pero
		// sugeriría que puede llegar cruda, y es justo lo que no debe volver a pasar.
		Vencimiento: fila.Vencimiento,
	}

	// ── LA FACTURA DE CONTADO NACE BLOQUEADA PARA PAGO ──────────────────────
	//
	// Decisión del Director Financiero (2026-09-09). El caso que la motiva: se compra en la
	// ferretería, se paga con el fondo de caja chica y el custodio registra el vale. La misma
	// factura llega después por correo, la ingesta la vuelve cuenta por pagar a nombre del
	// proveedor, y en paralelo la reposición del fondo crea un REINTEGRO al custodio por el mismo
	// monto: **el gasto sale dos veces por la puerta del banco**. Nada puede detectarlo hoy,
	// porque el vale de caja chica no tiene la clave del comprobante con la que cruzarlo.
	//
	// Bloqueada, la factura entra al expediente y se ve —el gasto queda registrado—, pero no puede
	// llegar al archivo de pagos sin que una persona la libere: las consultas que arman ese archivo
	// filtran por `NOT bloqueado_para_pago`.
	//
	// Solo aplica al camino XML, que es el único que sabe la condición de venta declarada por el
	// emisor. El .xlsx no trae `TipoDocumento`, y su columna «Condición» ya se usa nada más para
	// derivar el plazo.
	if fila.TipoDocumento != "" && strings.EqualFold(strings.TrimSpace(fila.Condicion), "contado") {
		in.BloqueadoParaPago = true
		in.BloqueoMotivo = "contado: confirmar si ya se pagó (caja chica, tarjeta o efectivo)"
	}
	return in, nil
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
