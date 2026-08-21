package bancos

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// Respuesta típica del web service del BCCR: XML de datos envuelto y escapado en <string>.
const respuestaBCCR = `<?xml version="1.0" encoding="utf-8"?>
<string xmlns="http://ws.sw.chc.bccr.fi.cr/">&lt;Datos_de_INGC011_CAT_INDICADORECONOMIC&gt;&lt;INGC011_CAT_INDICADORECONOMIC&gt;&lt;COD_INDICADORINTERNO&gt;318&lt;/COD_INDICADORINTERNO&gt;&lt;DES_FECHA&gt;2026-07-01T00:00:00-06:00&lt;/DES_FECHA&gt;&lt;NUM_VALOR&gt;515.2300&lt;/NUM_VALOR&gt;&lt;/INGC011_CAT_INDICADORECONOMIC&gt;&lt;/Datos_de_INGC011_CAT_INDICADORECONOMIC&gt;</string>`

func TestParseBCCR(t *testing.T) {
	v, err := parseBCCR([]byte(respuestaBCCR))
	if err != nil {
		t.Fatalf("parseBCCR error: %v", err)
	}
	if v.String() != "515.23" {
		t.Errorf("valor = %s, quiere 515.23", v.String())
	}
	// Sin cotización → error claro.
	if _, err := parseBCCR([]byte(`<string>&lt;Datos_de_INGC011_CAT_INDICADORECONOMIC&gt;&lt;/Datos_de_INGC011_CAT_INDICADORECONOMIC&gt;</string>`)); err == nil {
		t.Error("esperaba error cuando no hay NUM_VALOR")
	}
}

func TestBCCRClientObtener(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verifica que se envían las credenciales y el indicador.
		if r.URL.Query().Get("CorreoElectronico") == "" || r.URL.Query().Get("Token") == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(respuestaBCCR))
	}))
	defer srv.Close()

	f := NewBCCRClient(srv.URL, "correo@x.com", "tok", "318", 5*time.Second)
	if f == nil {
		t.Fatal("cliente nil con credenciales presentes")
	}
	v, err := f.Obtener(context.Background(), time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Obtener error: %v", err)
	}
	if v.String() != "515.23" {
		t.Errorf("valor = %s, quiere 515.23", v.String())
	}
	// Sin credenciales → fetcher nil (BCCR no configurado).
	if NewBCCRClient(srv.URL, "", "", "318", time.Second) != nil {
		t.Error("esperaba nil sin credenciales")
	}
}

func TestEsDiaDeSync(t *testing.T) {
	casos := map[string]bool{
		"2026-07-01": true,  // día 1
		"2026-07-15": true,  // día 15
		"2026-07-31": true,  // último de julio
		"2026-07-10": false, // día cualquiera
		"2026-02-28": true,  // último de febrero (no bisiesto)
		"2026-02-27": false,
	}
	for f, want := range casos {
		tt, _ := time.Parse("2006-01-02", f)
		if got := EsDiaDeSync(tt); got != want {
			t.Errorf("EsDiaDeSync(%s) = %v, quiere %v", f, got, want)
		}
	}
}
