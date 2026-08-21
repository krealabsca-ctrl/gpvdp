package bancos

// Cliente del "Servicio Web de Indicadores Económicos" del BCCR (§23).
// Diseño: la lógica de negocio depende de la INTERFAZ CotizacionFetcher, no del
// cliente HTTP concreto, de modo que sea stubbeable en tests y desactivable en
// entornos sin salida a internet/credenciales. El fetcher es nil si BCCR no está
// configurado; el service lo trata como ErrBCCRNoConfigurado (fallback manual).

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// BCCRSyncLog es un intento de sincronización (manual o automático) con su resultado.
type BCCRSyncLog struct {
	EmpresaID string `json:"empresa_id"`
	Fecha     string `json:"fecha"`     // YYYY-MM-DD objetivo
	Indicador string `json:"indicador"` // p. ej. 318 (venta)
	Valor     string `json:"valor"`     // "" si falló
	Exito     bool   `json:"exito"`
	Mensaje   string `json:"mensaje"`
	CreadoEn  string `json:"creado_en"`
}

// CotizacionFetcher obtiene la cotización de referencia del BCCR para una fecha.
type CotizacionFetcher interface {
	// Indicador que consulta (para registrar en la bitácora).
	Indicador() string
	// Obtener devuelve el valor del indicador para la fecha (o error si no disponible).
	Obtener(ctx context.Context, fecha time.Time) (decimal.Decimal, error)
}

// bccrClient es el fetcher real contra el web service del BCCR.
type bccrClient struct {
	wsURL     string
	email     string
	token     string
	indicador string
	http      *http.Client
}

// NewBCCRClient construye el fetcher del BCCR. Devuelve nil si faltan credenciales
// (correo/token) — el service lo interpreta como "no configurado" y no rompe el arranque.
func NewBCCRClient(wsURL, email, token, indicador string, timeout time.Duration) CotizacionFetcher {
	if email == "" || token == "" {
		return nil
	}
	if indicador == "" {
		indicador = "318"
	}
	return &bccrClient{
		wsURL:     wsURL,
		email:     email,
		token:     token,
		indicador: indicador,
		http:      &http.Client{Timeout: timeout},
	}
}

func (c *bccrClient) Indicador() string { return c.indicador }

// respuesta del web service: <Datos_de_...><INGC011_CAT_INDICADORECONOMIC><NUM_VALOR>…</NUM_VALOR>…
type bccrRespuesta struct {
	Valores []string `xml:"INGC011_CAT_INDICADORECONOMIC>NUM_VALOR"`
}

func (c *bccrClient) Obtener(ctx context.Context, fecha time.Time) (decimal.Decimal, error) {
	d := fecha.Format("02/01/2006") // el BCCR usa dd/mm/yyyy
	q := url.Values{}
	q.Set("Indicador", c.indicador)
	q.Set("FechaInicio", d)
	q.Set("FechaFinal", d)
	q.Set("Nombre", "GPVDP")
	q.Set("SubNiveles", "N")
	q.Set("CorreoElectronico", c.email)
	q.Set("Token", c.token)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.wsURL+"?"+q.Encode(), nil)
	if err != nil {
		return decimal.Zero, fmt.Errorf("bccr: armar request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return decimal.Zero, fmt.Errorf("bccr: llamada: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return decimal.Zero, fmt.Errorf("bccr: leer respuesta: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return decimal.Zero, fmt.Errorf("bccr: HTTP %d", resp.StatusCode)
	}
	return parseBCCR(body)
}

// parseBCCR extrae el primer NUM_VALOR de la respuesta XML del web service.
// El cuerpo suele venir como un string XML escapado dentro de un <string>…</string>;
// se desescapa y se parsea. Función pura para testeo.
func parseBCCR(body []byte) (decimal.Decimal, error) {
	s := strings.TrimSpace(string(body))
	// El ASMX envuelve el XML de datos en <string>…</string> (con entidades escapadas).
	if i := strings.Index(s, "<string"); i >= 0 {
		if j := strings.Index(s[i:], ">"); j >= 0 {
			inner := s[i+j+1:]
			if k := strings.LastIndex(inner, "</string>"); k >= 0 {
				inner = inner[:k]
			}
			s = html_unescape(inner)
		}
	}
	var r bccrRespuesta
	if err := xml.Unmarshal([]byte(s), &r); err != nil {
		return decimal.Zero, fmt.Errorf("bccr: XML inesperado: %w", err)
	}
	for _, v := range r.Valores {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		val, err := decimal.NewFromString(v)
		if err != nil {
			return decimal.Zero, fmt.Errorf("bccr: valor no numérico %q: %w", v, err)
		}
		return val, nil
	}
	return decimal.Zero, fmt.Errorf("bccr: sin cotización para la fecha")
}

// html_unescape desescapa las entidades XML/HTML mínimas que usa la envoltura del ASMX.
func html_unescape(s string) string {
	rep := strings.NewReplacer(
		"&lt;", "<", "&gt;", ">", "&amp;", "&", "&quot;", "\"", "&#xD;", "", "&#xA;", "",
	)
	return rep.Replace(s)
}
