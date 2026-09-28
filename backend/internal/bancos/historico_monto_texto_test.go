package bancos

// Montos escritos como TEXTO, y archivos que ni siquiera son un libro.
//
// Los dos defectos de acá se midieron contra la API real (POST /v1/bancos/importaciones/historico,
// Valle de Paz) antes de arreglarlos, no sobre una grilla inventada: el primero pedía una celda de
// TEXTO de verdad —una grilla de strings no distingue texto de número— y el segundo pedía subir un
// archivo que no es un .xlsx.

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// UN MONTO EN TEXTO CON PUNTO DE MILES NO PUEDE ENTRAR MIL VECES MÁS CHICO.
//
// El lector prefiere el valor CRUDO de la celda, y eso es correcto para una celda numérica. Pero
// una celda de TEXTO tiene el mismo valor crudo que formateado, así que el débito «25.000» —que en
// Costa Rica son veinticinco mil— se leía como el número 25,000 y entraba a los libros como ₡25,00.
// Medido contra la API real: `total_debitos` daba «25.00», con `errores: 0`. Mil veces menos plata
// y sin un solo aviso, que es exactamente lo que este importador no puede hacer, porque CREA
// movimientos en vez de solo etiquetarlos.
//
// El otro lector del módulo («Traer la clasificación desde Excel») siempre leyó ese mismo texto
// como 25000, así que además los dos contestaban distinto sobre el mismo archivo.
func TestUnMontoEnTextoConPuntoDeMilesNoEntraMilVecesMasChico(t *testing.T) {
	casos := []struct {
		nombre   string
		celda    any
		esperado string
	}{
		// El caso que se midió roto.
		{"texto «25.000» son veinticinco mil", "25.000", "25000"},
		{"texto «1.234» son mil doscientos treinta y cuatro", "1.234", "1234"},
		// Los que ya andaban bien y tienen que seguir igual.
		{"texto «25.000,00» con decimales", "25.000,00", "25000"},
		{"texto «1.234,56» a la costarricense", "1.234,56", "1234.56"},
		{"texto «1,234.56» a la inglesa", "1,234.56", "1234.56"},
		{"texto «CRC 1.500.000» como lo escribe el BP", "CRC 1.500.000", "1500000"},
		{"texto «25000» sin separadores", "25000", "25000"},
		// Un NÚMERO de verdad con tres decimales NO es un punto de miles: es el número, redondeado a
		// la precisión de los libros (numeric(16,2)). Lo que NO puede pasar es que salga 1234.
		{"número 1,234 con tres decimales", 1.234, "1.23"},
		{"número 25000 de verdad", 25000.0, "25000"},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			libro := libroPrueba{filas: [][]any{
				{"Movimientos de Valle de Paz"}, {"del 01/01/2025 al 31/12/2025"},
				encabezadoExport,
				filaExport("15/01/2025", "UNICA", c.celda, nil, "MONTO EN TEXTO",
					"Davivienda Colones", "", ""),
			}}
			lec := leerLibro(t, libro.bytes(t))
			if len(lec.Errores) != 0 {
				t.Fatalf("errores = %+v, no se esperaba ninguno", lec.Errores)
			}
			if len(lec.Filas) != 1 {
				t.Fatalf("filas = %d, se esperaba 1", len(lec.Filas))
			}
			if got := lec.Filas[0].Debito.String(); got != c.esperado {
				t.Errorf("débito = %s, se esperaba %s", got, c.esperado)
			}
		})
	}
}

// UN ARCHIVO QUE NO ES UN .XLSX SE RECHAZA CON INSTRUCCIONES, NO CON «ERROR INTERNO».
//
// Medido contra la API real antes del arreglo: subir un .csv —o cualquier archivo con nombre
// .xlsx— devolvía 500 {"code":"ERROR_INTERNO","message":"error interno"}. El usuario no puede
// distinguir «me equivoqué de archivo» de «se cayó el servidor», así que reintenta el mismo
// archivo. El contrato de este endpoint promete 400 VALIDACION «no se pudo leer el archivo».
func TestUnArchivoQueNoEsUnLibroSeRechazaConInstrucciones(t *testing.T) {
	// 1) El lector devuelve el centinela y no un error suelto.
	_, _, _, _, _, err := gridDeArchivoHistorico([]byte("esto no es un excel\n"), hojaMovimientos)
	if err == nil {
		t.Fatal("abrir basura no dio error")
	}
	if !errors.Is(err, ErrHistoricoArchivoIlegible) {
		t.Fatalf("el error no es ErrHistoricoArchivoIlegible: %v", err)
	}

	// 2) Y el borde HTTP lo traduce a 400 con qué hacer, sin el detalle de excelize.
	gin.SetMode(gin.TestMode)
	h := NewHandler(nil, zap.NewNop())
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/bancos/importaciones/historico", nil)

	h.responderError(ctx, err, "test")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, se esperaba 400 — %s", rec.Code, rec.Body.String())
	}
	var cuerpo struct{ Code, Message string }
	if e := json.Unmarshal(rec.Body.Bytes(), &cuerpo); e != nil {
		t.Fatalf("cuerpo ilegible: %v — %s", e, rec.Body.String())
	}
	if cuerpo.Code != "VALIDACION" {
		t.Fatalf("code = %q, se esperaba VALIDACION — %s", cuerpo.Code, rec.Body.String())
	}
	if !strings.Contains(cuerpo.Message, "no se pudo leer el archivo") ||
		!strings.Contains(cuerpo.Message, ".xlsx") {
		t.Errorf("el mensaje no dice qué hacer: %q", cuerpo.Message)
	}
	// El detalle de excelize sirve en el log, no en la pantalla del Director.
	if strings.Contains(cuerpo.Message, "zip") {
		t.Errorf("el mensaje filtra el detalle interno: %q", cuerpo.Message)
	}
}
