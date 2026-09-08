package inventario

// El BORDE del inventario: que cada error de dominio salga con su estado HTTP.
//
// Este test existe porque el defecto ya ocurrió tres veces en este proyecto: se agrega un centinela
// nuevo, se olvida el caso en el `switch` de `responder`, y una operación rechazada por una regla de
// negocio sale como **500 «error interno»**. El usuario no puede distinguir «te falta explicar dos
// diferencias» de «el servidor se cayó», así que reintenta lo mismo.
//
// La lista de abajo es la red: agregar un centinela sin agregarlo acá deja el test rojo.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// zapNop es un logger que descarta todo: el caso default de responder() loguea, y un test no debe
// ensuciar la salida con el error que está probando a propósito.
func zapNop() *zap.Logger { return zap.NewNop() }

// estadoDe corre el traductor de errores y devuelve el estado y el cuerpo.
func estadoDe(t *testing.T, err error) (int, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	h := &Handler{log: zapNop()}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/inventario/x", nil)
	h.responder(c, err, "test")
	return w.Code, w.Body.String()
}

func TestNingunErrorDeDominioSaleComo500(t *testing.T) {
	casos := []struct {
		nombre string
		err    error
		quiere int
	}{
		// No encontrado
		{"artículo", ErrArticuloNoEncontrado, http.StatusNotFound},
		{"categoría", ErrCategoriaNoEncontrada, http.StatusNotFound},
		{"unidad", ErrUnidadNoEncontrada, http.StatusNotFound},
		{"traslado", ErrTrasladoNoEncontrado, http.StatusNotFound},
		{"servicio", ErrServicioNoEncontrado, http.StatusNotFound},
		{"conteo", ErrConteoNoEncontrado, http.StatusNotFound},
		{"línea de conteo", ErrLineaNoEncontrada, http.StatusNotFound},
		// Validación de borde
		{"nombre", ErrNombreRequerido, http.StatusBadRequest},
		{"código", ErrCodigoRequerido, http.StatusBadRequest},
		{"modo", ErrModoInvalido, http.StatusBadRequest},
		{"cantidad", ErrCantidadInvalida, http.StatusBadRequest},
		{"costo", ErrCostoNegativo, http.StatusBadRequest},
		{"sede", ErrSedeRequerida, http.StatusBadRequest},
		{"fecha", ErrFechaInvalida, http.StatusBadRequest},
		{"misma sede", ErrMismaSede, http.StatusBadRequest},
		// Reglas de negocio
		{"duplicado", ErrDuplicado, http.StatusConflict},
		{"motivo", ErrMotivoRequerido, http.StatusUnprocessableEntity},
		{"traslado ya recibido", ErrTrasladoYaRecibido, http.StatusUnprocessableEntity},
		{"conteo ya cerrado", ErrConteoYaCerrado, http.StatusUnprocessableEntity},
		{"conteo abierto en la sede", ErrConteoAbiertoEnLaSede, http.StatusUnprocessableEntity},
		{"conteo sin líneas", ErrConteoSinLineas, http.StatusUnprocessableEntity},
		{"motivo para anular", ErrMotivoAnularRequerido, http.StatusUnprocessableEntity},
		{"servicio sin consumos", ErrServicioSinConsumos, http.StatusUnprocessableEntity},
		// Consignación (Fase 3)
		{"consignada sin proveedor", ErrProveedorConsignadaRequerido, http.StatusUnprocessableEntity},
		{"no es consignada", ErrNoEsConsignada, http.StatusUnprocessableEntity},
		{"consignada en bodega", ErrConsignadaEnBodega, http.StatusUnprocessableEntity},
		{"consignada en tránsito", ErrConsignadaEnTransito, http.StatusUnprocessableEntity},
		{"consignada sin provisión", ErrConsignadaSinFactura, http.StatusUnprocessableEntity},
		{"la real es la provisión", ErrDocumentoRealEsLaProvision, http.StatusUnprocessableEntity},
		{"consignada ya facturada", ErrConsignadaYaFacturada, http.StatusConflict},
		{"ya conciliada", ErrYaConciliada, http.StatusConflict},
		// Los rechazos que protegen la plata (tanda 1 de la revisión adversarial).
		{"costo cero", ErrCostoCeroNoSeFactura, http.StatusUnprocessableEntity},
		{"salida no facturable sola", ErrSalidaNoFacturableSola, http.StatusUnprocessableEntity},
		{"devuelta no se factura", ErrDevueltaNoSeFactura, http.StatusUnprocessableEntity},
		{"factura de otro proveedor", ErrDocumentoRealDeOtroProveedor, http.StatusUnprocessableEntity},
		{"factura anulada", ErrDocumentoRealAnulado, http.StatusUnprocessableEntity},
		{"factura no existe en la empresa", ErrDocumentoRealNoEncontrado, http.StatusNotFound},
		{"factura ya enlazada", ErrDocumentoRealYaEnlazado, http.StatusConflict},
		// Los que vienen de CxP traducidos por el adaptador. Que estén acá es el arreglo del defecto
		// que ya ocurrió CINCO veces: un error ajeno sin case sale como 500 y el usuario no puede
		// distinguir «esa factura ya existe» de un servidor caído.
		{"provisión ya existe en CxP", ErrProvisionYaExisteEnCxP, http.StatusConflict},
		{"rechazo de CxP", ErrCxPRechazo, http.StatusUnprocessableEntity},
		{"situación inválida", ErrSituacionInvalida, http.StatusBadRequest},
		{"falta el documento real", ErrDocumentoRealRequerido, http.StatusBadRequest},
		{"estado de unidad inválido", ErrEstadoUnidadInvalido, http.StatusBadRequest},
		// Sin CxP conectado no es culpa del usuario: el pedido está bien y falta una pieza del
		// entorno, así que 503 y no 500.
		{"sin facturador de CxP", ErrSinFacturadorCxP, http.StatusServiceUnavailable},
		// Los que llevan datos
		{"sin existencia", &SinExistenciaError{Articulo: "Urna", Sede: "Cartago", Hay: 4, Pedido: 9}, http.StatusUnprocessableEntity},
		{"unidad no disponible", &UnidadNoDisponibleError{Numero: "CF-1", Estado: EstadoUsada, Quiere: "usarla"}, http.StatusUnprocessableEntity},
		{"unidad en otra sede", &UnidadEnOtraSedeError{Numero: "CF-1", Esta: "Sabana", Pedida: "Liberia"}, http.StatusUnprocessableEntity},
		{"diferencias sin explicar", &DiferenciasSinExplicarError{Cuantas: 2, Articulos: []string{"Urna", "Cofre"}}, http.StatusUnprocessableEntity},
		{"líneas sin contar", &SinContarError{Cuantas: 1, Total: 4}, http.StatusUnprocessableEntity},
		{"consignación por cantidad", &ConsignacionSoloPorUnidadError{Articulo: "Urna mármol"}, http.StatusUnprocessableEntity},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			got, cuerpo := estadoDe(t, c.err)
			if got == http.StatusInternalServerError {
				t.Fatalf("%v salió como 500 «error interno»: falta su caso en el switch de responder()", c.err)
			}
			if got != c.quiere {
				t.Errorf("estado = %d, se esperaba %d (%s)", got, c.quiere, cuerpo)
			}
			if strings.Contains(cuerpo, "error interno") {
				t.Errorf("el mensaje no debería decir «error interno»: %s", cuerpo)
			}
		})
	}
}

func TestElMensajeDelCierreDiceQueFalta(t *testing.T) {
	// «No se puede cerrar» a secas obliga a revisar 40 líneas a mano para encontrar las dos que
	// faltan. Los dos rechazos del cierre tienen que nombrar el problema con números.
	_, cuerpo := estadoDe(t, &SinContarError{Cuantas: 3, Total: 12})
	for _, esperado := range []string{"3", "12", "sin contar"} {
		if !strings.Contains(cuerpo, esperado) {
			t.Errorf("el mensaje no dice %q: %s", esperado, cuerpo)
		}
	}

	_, cuerpo2 := estadoDe(t, &DiferenciasSinExplicarError{
		Cuantas: 2, Articulos: []string{"Urna mármol", "Cofre roble"},
	})
	for _, esperado := range []string{"Urna mármol", "Cofre roble", "motivo"} {
		if !strings.Contains(cuerpo2, esperado) {
			t.Errorf("el mensaje no dice %q: %s", esperado, cuerpo2)
		}
	}
}

func TestConMuchasDiferenciasElMensajeNoSeVuelveUnaLista(t *testing.T) {
	// Con 20 artículos, listarlos todos hace un mensaje que nadie lee. Se nombran los primeros y se
	// dice cuántos más quedan.
	muchos := []string{"A", "B", "C", "D", "E", "F"}
	e := &DiferenciasSinExplicarError{Cuantas: len(muchos), Articulos: muchos}
	msg := e.Error()
	if !strings.Contains(msg, "y 3 más") {
		t.Errorf("debería resumir la cola: %s", msg)
	}
	if strings.Contains(msg, "F") {
		t.Errorf("no debería listar los seis: %s", msg)
	}
}
