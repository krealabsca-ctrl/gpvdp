package cxp

// El envío del comprobante: la copia al aprobador y la bitácora.
//
// Lo que estos casos protegen es, sobre todo, lo que ANTES NO PASABA: un envío que falla dejaba el
// error en la pantalla y se perdía al recargar, así que nadie podía contestar «¿cuándo y a qué
// dirección se le mandó el comprobante a este proveedor, y por qué no llegó?».

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/gpvdp/erp/internal/shared"
)

// mailerFalso sustituye al transporte: así se puede probar el camino del envío FALLIDO sin un
// servidor de correo, que es justo el camino que no estaba probado.
type mailerFalso struct {
	sobres    []shared.Sobre
	empresas  []string
	origen    string
	remitente string
	err       error
}

func (m *mailerFalso) EnviarConAdjunto(_ context.Context, empresaID string, sobre shared.Sobre) (string, string, error) {
	m.empresas = append(m.empresas, empresaID)
	m.sobres = append(m.sobres, sobre)
	return m.origen, m.remitente, m.err
}

// comprobanteListo es una factura pagada con su PDF adjunto, proveedor con correo y aprobador.
func comprobanteListo() ComprobanteEnvio {
	return ComprobanteEnvio{
		Comprobante:     Comprobante{Filename: "comprobante-BAC-1234.pdf", Mime: "application/pdf", Contenido: []byte("%PDF-1.4")},
		ProveedorEmail:  "proveedor@ferreteria.cr",
		ProveedorNombre: "Ferretería La Esquina",
		Consecutivo:     "00100001010000000123",
		Moneda:          "CRC", Total: "125000.00", TotalCRC: "125000.00",
		AprobadorEmail:  "director@valledepazcr.com",
		AprobadorNombre: "Ana Rojas",
		SubidoEn:        time.Date(2026, 9, 16, 9, 0, 0, 0, time.UTC),
	}
}

func servicioDeEnvio(envio ComprobanteEnvio, mailer *mailerFalso) (*Service, *fakeRepo) {
	repo := &fakeRepo{compEnvio: envio}
	s := NewService(repo, nil, zap.NewNop())
	s.SetMailer(mailer)
	return s, repo
}

// ─────────────────────────────────────────────────────────────────────────────

// TestElComprobanteVaConCopiaOcultaAlAprobador: la decisión del Director Financiero del
// 17-set-2026, con la copia OCULTA para no publicarle a 649 proveedores externos la dirección de
// quien autoriza los pagos.
func TestElComprobanteVaConCopiaOcultaAlAprobador(t *testing.T) {
	mailer := &mailerFalso{origen: "EMPRESA", remitente: "cxp@valledepazcr.com"}
	svc, repo := servicioDeEnvio(comprobanteListo(), mailer)

	res, err := svc.EnviarComprobante(context.Background(), "e1", "d1", "u1")
	if err != nil {
		t.Fatalf("el envío tenía que salir bien: %v", err)
	}
	if len(mailer.sobres) != 1 {
		t.Fatalf("tenía que mandarse un correo, se mandaron %d", len(mailer.sobres))
	}
	sobre := mailer.sobres[0]
	if sobre.Para != "proveedor@ferreteria.cr" {
		t.Fatalf("destinatario = %q", sobre.Para)
	}
	if sobre.CopiaOculta != "director@valledepazcr.com" {
		t.Fatalf("la copia al aprobador tenía que ir oculta y con su dirección, fue %q", sobre.CopiaOculta)
	}
	if sobre.Adjunto == nil || sobre.Adjunto.Filename != "comprobante-BAC-1234.pdf" {
		t.Fatal("el comprobante tenía que viajar adjunto")
	}
	if mailer.empresas[0] != "e1" {
		t.Fatalf("el buzón se resuelve POR EMPRESA: se pidió el de %q", mailer.empresas[0])
	}
	// Y quedó la constancia.
	if len(repo.envios) != 1 {
		t.Fatalf("tenía que quedar una fila de bitácora, quedaron %d", len(repo.envios))
	}
	fila := repo.envios[0]
	if fila.Resultado != EnvioOK || fila.Copia != "director@valledepazcr.com" ||
		fila.Destinatario != "proveedor@ferreteria.cr" || fila.Origen != "EMPRESA" {
		t.Fatalf("la bitácora no dice lo que pasó: %+v", fila)
	}
	if fila.ErrorCategoria != "" {
		t.Fatalf("un envío OK no puede traer categoría de error (lo prohíbe un CHECK): %q", fila.ErrorCategoria)
	}
	if fila.Archivo != "comprobante-BAC-1234.pdf" || fila.ComprobanteSubidoEn == nil {
		t.Fatalf("la bitácora tiene que decir QUÉ archivo se mandó: %+v", fila)
	}
	// Y la respuesta dice a quién se le mandó, que es lo que la pantalla necesita.
	if res.Destinatario != "proveedor@ferreteria.cr" || res.Copia != "director@valledepazcr.com" {
		t.Fatalf("la respuesta no dice a quién se le mandó: %+v", res)
	}
}

// TestSinAprobadorElEnvioNoSeCae: la copia es accesoria. Hoy hay facturas sin aprobación
// registrada, y trancar el comprobante por eso sería castigar al proveedor por un hueco interno.
func TestSinAprobadorElEnvioNoSeCae(t *testing.T) {
	envio := comprobanteListo()
	envio.AprobadorEmail, envio.AprobadorNombre = "", ""
	mailer := &mailerFalso{origen: "EMPRESA", remitente: "cxp@valledepazcr.com"}
	svc, repo := servicioDeEnvio(envio, mailer)

	res, err := svc.EnviarComprobante(context.Background(), "e1", "d1", "u1")
	if err != nil {
		t.Fatalf("el envío NO se puede caer por falta de aprobador: %v", err)
	}
	if mailer.sobres[0].CopiaOculta != "" {
		t.Fatalf("no había a quién copiar: %q", mailer.sobres[0].CopiaOculta)
	}
	if repo.envios[0].Copia != "" || repo.envios[0].Resultado != EnvioOK {
		t.Fatalf("tenía que quedar escrito que salió sin copia: %+v", repo.envios[0])
	}
	if res.Copia != "" {
		t.Fatalf("la respuesta tiene que decir que no hubo copia: %+v", res)
	}
}

// TestUnEnvioFallidoQuedaEnLaBitacora es el agujero que esta tarea viene a tapar.
func TestUnEnvioFallidoQuedaEnLaBitacora(t *testing.T) {
	const password = "hunter2-del-buzon"
	mailer := &mailerFalso{
		origen: "EMPRESA", remitente: "cxp@valledepazcr.com",
		err: &shared.ErrorSMTP{Categoria: shared.CategoriaAutenticacion, Codigo: 535, Servidor: "smtp.office365.com:587"},
	}
	svc, repo := servicioDeEnvio(comprobanteListo(), mailer)

	_, err := svc.EnviarComprobante(context.Background(), "e1", "d1", "u1")
	if err == nil {
		t.Fatal("el envío tenía que fallar")
	}
	if len(repo.envios) != 1 {
		t.Fatalf("un envío FALLIDO también deja fila; quedaron %d", len(repo.envios))
	}
	fila := repo.envios[0]
	if fila.Resultado != EnvioError {
		t.Fatalf("resultado = %q, se esperaba %q", fila.Resultado, EnvioError)
	}
	if fila.ErrorCategoria != shared.CategoriaAutenticacion {
		t.Fatalf("categoría = %q, se esperaba %q", fila.ErrorCategoria, shared.CategoriaAutenticacion)
	}
	if fila.Error == "" {
		t.Fatal("la fila tiene que decir POR QUÉ falló, en palabras que el operador entienda")
	}
	// El texto que se guarda para siempre no lleva ni la contraseña, ni el host, ni el usuario del
	// buzón: esta bitácora la leen siete roles y la configuración del correo, uno.
	for _, prohibido := range []string{password, "smtp.office365.com", "cxp@valledepazcr.com", "535"} {
		if strings.Contains(fila.Error, prohibido) {
			t.Fatalf("la bitácora no puede guardar %q: %q", prohibido, fila.Error)
		}
	}
	// Y sigue siendo un dato útil: el destinatario y la copia quedan igual.
	if fila.Destinatario != "proveedor@ferreteria.cr" || fila.Copia != "director@valledepazcr.com" {
		t.Fatalf("hasta el intento fallido dice a quién se le quiso mandar: %+v", fila)
	}
}

// TestElOrigenGuardadoSiempreEsValido: el CHECK de la base solo acepta EMPRESA o GLOBAL. Un INSERT
// rebotado acá borraría la única evidencia justo cuando el envío falló.
func TestElOrigenGuardadoSiempreEsValido(t *testing.T) {
	// El resolver ni llegó a decidir: devuelve origen vacío junto con el error.
	mailer := &mailerFalso{origen: "", err: ErrCorreoNoConfigurado}
	svc, repo := servicioDeEnvio(comprobanteListo(), mailer)
	if _, err := svc.EnviarComprobante(context.Background(), "e1", "d1", "u1"); err == nil {
		t.Fatal("tenía que fallar")
	}
	if repo.envios[0].Origen != "GLOBAL" {
		t.Fatalf("origen = %q; tiene que ser EMPRESA o GLOBAL", repo.envios[0].Origen)
	}
	if repo.envios[0].ErrorCategoria != "CORREO_NO_CONFIGURADO" {
		t.Fatalf("categoría = %q", repo.envios[0].ErrorCategoria)
	}
}

// TestElReenvioSeDistingue: reenviar es la misma llamada, y la bitácora dice cuál fue el primero.
func TestElReenvioSeDistingue(t *testing.T) {
	mailer := &mailerFalso{origen: "EMPRESA", remitente: "cxp@valledepazcr.com"}
	svc, repo := servicioDeEnvio(comprobanteListo(), mailer)
	repo.reenvio = true // el repositorio ya tenía un OK anterior de este documento

	res, err := svc.EnviarComprobante(context.Background(), "e1", "d1", "u1")
	if err != nil {
		t.Fatalf("reenviar: %v", err)
	}
	if !res.Reenvio {
		t.Fatal("la respuesta tenía que decir que fue un reenvío")
	}
}

// TestProveedorSinCorreoNoDejaFila: no hubo intento de envío. La fila exigiría un destinatario que
// no existe y una causa que no es del servidor de correo, sino de la ficha del proveedor.
func TestProveedorSinCorreoNoDejaFila(t *testing.T) {
	envio := comprobanteListo()
	envio.ProveedorEmail = ""
	mailer := &mailerFalso{origen: "EMPRESA"}
	svc, repo := servicioDeEnvio(envio, mailer)

	if _, err := svc.EnviarComprobante(context.Background(), "e1", "d1", "u1"); !errors.Is(err, ErrProveedorSinEmail) {
		t.Fatalf("se esperaba ErrProveedorSinEmail, fue %v", err)
	}
	if len(repo.envios) != 0 {
		t.Fatalf("no tenía que quedar fila: %+v", repo.envios)
	}
	if len(mailer.sobres) != 0 {
		t.Fatal("no tenía que intentarse ningún envío")
	}
}

// TestSinMailerNoSeRevienta: el servicio puede construirse sin mailer (tests, arranque a medias).
func TestSinMailerNoSeRevienta(t *testing.T) {
	svc := NewService(&fakeRepo{compEnvio: comprobanteListo()}, nil, zap.NewNop())
	if _, err := svc.EnviarComprobante(context.Background(), "e1", "d1", "u1"); !errors.Is(err, ErrCorreoNoConfigurado) {
		t.Fatalf("se esperaba ErrCorreoNoConfigurado, fue %v", err)
	}
}

// TestCategoriaDeEnvioSoloDevuelveValoresQueLaBaseAcepta: la lista tiene que coincidir con el
// CHECK de comprobante_envio.error_categoria. Si se desincronizan, el INSERT de la bitácora rebota
// justo cuando el envío ya falló — y se pierde la evidencia.
func TestCategoriaDeEnvioSoloDevuelveValoresQueLaBaseAcepta(t *testing.T) {
	permitidas := map[string]bool{
		"AUTENTICACION_RECHAZADA": true, "HOST_INALCANZABLE": true, "RELAY_DENEGADO": true,
		"TLS_FALLIDO": true, "TIEMPO_AGOTADO": true, "CORREO_NO_CONFIGURADO": true,
		"SECRETO_ILEGIBLE": true, "OTRO": true,
	}
	casos := []struct {
		err    error
		quiere string
	}{
		{ErrCorreoNoConfigurado, "CORREO_NO_CONFIGURADO"},
		{ErrCifradoNoDisponible, "SECRETO_ILEGIBLE"},
		{ErrSecretoCorreoIlegible, "SECRETO_ILEGIBLE"},
		{&shared.ErrorSMTP{Categoria: shared.CategoriaAutenticacion}, "AUTENTICACION_RECHAZADA"},
		{&shared.ErrorSMTP{Categoria: shared.CategoriaHost}, "HOST_INALCANZABLE"},
		{&shared.ErrorSMTP{Categoria: shared.CategoriaRelay}, "RELAY_DENEGADO"},
		{&shared.ErrorSMTP{Categoria: shared.CategoriaTLS}, "TLS_FALLIDO"},
		{&shared.ErrorSMTP{Categoria: shared.CategoriaTimeout}, "TIEMPO_AGOTADO"},
		{errors.New("algo que nadie previó"), "OTRO"},
	}
	for _, caso := range casos {
		cat := categoriaDeEnvio(caso.err)
		if cat != caso.quiere {
			t.Fatalf("categoría de %v = %q, se esperaba %q", caso.err, cat, caso.quiere)
		}
		if !permitidas[cat] {
			t.Fatalf("la base NO acepta la categoría %q", cat)
		}
	}
}

// TestLosErroresDelCorreoNoSalenComo500 — la advertencia explícita de esta tarea: un centinela
// nuevo que no esté en el switch sale como 500 «error interno» y el usuario no lee el motivo.
func TestLosErroresDelCorreoNoSalenComo500(t *testing.T) {
	casos := []struct {
		nombre string
		err    error
		quiere int
	}{
		{"sin comprobante adjunto", ErrComprobanteNoEncontrado, http.StatusNotFound},
		{"factura no pagada", ErrDocNoPagado, http.StatusConflict},
		{"proveedor sin correo", ErrProveedorSinEmail, http.StatusUnprocessableEntity},
		{"correo sin configurar", ErrCorreoNoConfigurado, http.StatusUnprocessableEntity},
		{"falta CIFRADO_SECRET", ErrCifradoNoDisponible, http.StatusUnprocessableEntity},
		{"secreto ilegible", ErrSecretoCorreoIlegible, http.StatusUnprocessableEntity},
		{"el servidor rechazó", &shared.ErrorSMTP{Categoria: shared.CategoriaAutenticacion, Codigo: 535, Servidor: "smtp.office365.com:587"}, http.StatusUnprocessableEntity},
		{"relay denegado", &shared.ErrorSMTP{Categoria: shared.CategoriaRelay, Codigo: 550, Servidor: "smtp.office365.com:587"}, http.StatusUnprocessableEntity},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/cxp/documentos/d1/comprobante/enviar", nil)
			h := &Handler{log: zap.NewNop()}
			h.responderError(c, caso.err, "enviar-comprobante")
			if w.Code != caso.quiere {
				t.Fatalf("estado = %d, se esperaba %d (cuerpo: %s)", w.Code, caso.quiere, w.Body.String())
			}
			if w.Code == http.StatusInternalServerError {
				t.Fatal("un rechazo con motivo NUNCA puede salir como «error interno»")
			}
		})
	}
}

// TestElMensajeDeSMTPQueVeElUsuarioNoDelataElServidor: el 422 de CxP lleva la frase, no el detalle
// técnico. El detalle vive en correo_saliente.probado_error, detrás de admin.correo.
func TestElMensajeDeSMTPQueVeElUsuarioNoDelataElServidor(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/cxp/documentos/d1/comprobante/enviar", nil)
	h := &Handler{log: zap.NewNop()}
	h.responderError(c, &shared.ErrorSMTP{
		Categoria: shared.CategoriaAutenticacion, Codigo: 535, Servidor: "smtp.office365.com:587",
	}, "enviar-comprobante")

	cuerpo := w.Body.String()
	for _, prohibido := range []string{"smtp.office365.com", "535", "AUTENTICACION_RECHAZADA"} {
		if strings.Contains(cuerpo, prohibido) {
			t.Fatalf("la respuesta de CxP no puede traer %q: %s", prohibido, cuerpo)
		}
	}
	if !strings.Contains(cuerpo, "credenciales") {
		t.Fatalf("el usuario tiene que leer qué pasó y a quién avisarle: %s", cuerpo)
	}
}
