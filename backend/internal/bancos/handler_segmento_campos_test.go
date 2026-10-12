package bancos

// Lo que el servicio calcula tiene que LLEGAR al navegador (5-oct-2026).
//
// El Director buscó el movimiento 690716201 en «Mi partida» con setiembre activo. El movimiento es
// del 25 de agosto, está en su partida (Deposito de Clientes) y es un crédito: el aviso «también
// aparece en Agosto 2026» tenía que salir. La pantalla le respondió «Ningún movimiento de tu partida
// coincide con lo que filtraste».
//
// El cálculo estaba bien —la misma consulta a mano devolvía «2026-08 | 1»— y la pantalla sabía
// pintarlo. Lo que faltaba era el último tramo: el handler arma la respuesta con un `gin.H` campo
// por campo y nunca nombró `otros_meses`, así que el dato moría al serializar. Las pruebas del
// repositorio pasaban porque llaman a la función directamente.
//
// Es el mismo modo de falla que los centinelas que salían como 500: probar que una función devuelve
// lo correcto NO prueba que el usuario lo reciba. Por eso estas dos pruebas van por HTTP, y la
// segunda compara el JSON contra los tags del struct para que el PRÓXIMO campo no se caiga igual.

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func TestElAvisoDeOtroMesLlegaPorHTTP(t *testing.T) {
	repo, svc, _ := baseMiPartida(t)
	ctx := context.Background()

	// El caso del Director, con sus datos: un crédito de agosto, en su partida, buscado por el
	// número de documento mientras la pantalla muestra setiembre.
	if _, err := repo.pool.Exec(ctx, `
		INSERT INTO movimiento_bancario
			(empresa_id, cuenta_bancaria_id, fecha, documento, descripcion, debito, credito,
			 moneda_original, monto_original, monto_crc, concepto_id, clasificacion_id,
			 estado_clasificacion, natural_key, incluido)
		VALUES ($1::uuid, $2::uuid, '2026-08-25'::date, '690716201', 'DR/CR LINEA SINPE', 0, 90000,
		        'CRC', 90000, 90000, $3::uuid, $4::uuid, 'REVISADO', 'nk-690716201', true)`,
		empresaPrueba, cuentaBuena, conceptoID, partidaMia); err != nil {
		t.Fatalf("sembrar el movimiento de agosto: %v", err)
	}

	code, cuerpo := pedir(t, routerReal(svc, usuarioPrueba), http.MethodGet,
		"/v1/bancos/mi-segmento/movimientos?q=690716201&periodo=2026-09", "")
	if code != http.StatusOK {
		t.Fatalf("status = %d: %s", code, cuerpo)
	}

	var res struct {
		Movimientos struct {
			Total int `json:"total"`
		} `json:"movimientos"`
		OtrosMeses []CoincidenciaEnOtroMes `json:"otros_meses"`
	}
	if err := json.Unmarshal([]byte(cuerpo), &res); err != nil {
		t.Fatalf("cuerpo ilegible: %v — %s", err, cuerpo)
	}
	// En setiembre no hay nada: eso está bien, el movimiento es de agosto.
	if res.Movimientos.Total != 0 {
		t.Errorf("setiembre trajo %d movimientos, se esperaba 0", res.Movimientos.Total)
	}
	// Y justamente por eso el aviso tiene que viajar: sin él, la pantalla afirma que no existe.
	if len(res.OtrosMeses) == 0 {
		t.Fatalf("el JSON no trae «otros_meses»: el movimiento está a un mes de distancia y la pantalla dice que no coincide nada.\ncuerpo: %s", cuerpo)
	}
	if res.OtrosMeses[0].Periodo != "2026-08" || res.OtrosMeses[0].Cuantos != 1 {
		t.Errorf("otros_meses = %+v, se esperaba 2026-08 con 1", res.OtrosMeses)
	}
}

// Todo campo de MiSegmento tiene que salir en la respuesta. El handler arma un mapa a mano, y un
// mapa no se entera de un campo nuevo: hay que compararlo con el struct o el próximo dato calculado
// vuelve a morir en silencio entre el servicio y el navegador.
func TestLaRespuestaTraeTodosLosCamposDeMiSegmento(t *testing.T) {
	_, svc, _ := baseMiPartida(t)

	code, cuerpo := pedir(t, routerReal(svc, usuarioPrueba), http.MethodGet,
		"/v1/bancos/mi-segmento/movimientos?periodo=2026-09", "")
	if code != http.StatusOK {
		t.Fatalf("status = %d: %s", code, cuerpo)
	}
	var enJSON map[string]json.RawMessage
	if err := json.Unmarshal([]byte(cuerpo), &enJSON); err != nil {
		t.Fatalf("cuerpo ilegible: %v — %s", err, cuerpo)
	}

	tipo := reflect.TypeOf(MiSegmento{})
	revisados := 0
	for i := 0; i < tipo.NumField(); i++ {
		tag := tipo.Field(i).Tag.Get("json")
		nombre := strings.Split(tag, ",")[0]
		if nombre == "" || nombre == "-" {
			continue
		}
		revisados++
		if _, hay := enJSON[nombre]; !hay {
			t.Errorf("el campo %s (%s) se calcula en el servicio y NO sale en la respuesta: el handler arma el mapa campo por campo y hay que agregarlo ahí",
				tipo.Field(i).Name, nombre)
		}
	}
	// Si el struct se vaciara, la prueba pasaría sin revisar nada y no protegería de nada.
	if revisados < 5 {
		t.Fatalf("solo se revisaron %d campos de MiSegmento: la prueba dejó de medir", revisados)
	}
}
