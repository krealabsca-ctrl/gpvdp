package bancos

// Tests del descubridor de patrones. Los casos usan descripciones REALES de los estados de
// cuenta de Valle de Paz (julio 2026), porque el valor de esto es que separe bien los hechos
// que de verdad ocurren.

import (
	"strings"
	"testing"

	"github.com/shopspring/decimal"
)

// Descripciones reales, recortadas a lo que importa para la forma.
const (
	smo1 = "DR/CR LINEA SINPE (SMO-2026063081483000918909207 -  TRANSFERENCIA SINP)"
	smo2 = "DR/CR LINEA SINPE (SMO-2026063010283001715983001 - 118850490_VALERIA_RO)"
	smo3 = "DR/CR LINEA SINPE (SMO-2026063015283009886534357 - PAGO JUNIO 2026)"
	sal1 = "DR/CR LINEA SINPE (2026070110422000005554535 - 992115834 Pagar Proveedores ADELANTO VIATICOS)"
	sal2 = "DR/CR LINEA SINPE (2026070110422000005554541 - 992115834 Pagar Proveedores ADELANTO VIATICOS)"
	mas1 = "DEBITO MASIVO SINPE (2026070110431000132373477 - 88093552 - SEGUROS LAFISE COSTA RICA S A)"
	mas2 = "DEBITO MASIVO SINPE (2026070310431000132485798 - CINDY MARIANA ZELEDON PEREZ)"
	cta1 = "Credito en Cuenta 3101318985 NOEMYVALVERDEGAMBOA VALLE DE PAZ 071326000037436708000062"
	cta2 = "Credito en Cuenta 3101318985 JOSUEPICADOCHAVARRIA VALLE DE PAZ 071326000037436708000032"
)

func credito(desc string) LineaSinClasificar {
	return LineaSinClasificar{Descripcion: desc, EsDebito: false, Monto: decimal.RequireFromString("10000")}
}

func debito(desc string) LineaSinClasificar {
	return LineaSinClasificar{Descripcion: desc, EsDebito: true, Monto: decimal.RequireFromString("50000")}
}

// repetir crea n copias de una línea variando el número, como en el banco real.
func repetir(base LineaSinClasificar, n int) []LineaSinClasificar {
	out := make([]LineaSinClasificar, 0, n)
	for i := 0; i < n; i++ {
		l := base
		l.Descripcion = strings.Replace(base.Descripcion, "2026063081483", "202606308148"+string(rune('0'+i%10)), 1)
		out = append(out, l)
	}
	return out
}

func TestFormaDescripcion(t *testing.T) {
	casos := []struct{ desc, quiere string }{
		{smo1, "dr/cr linea sinpe (smo-#"},
		{smo2, "dr/cr linea sinpe (smo-#"},
		{sal1, "dr/cr linea sinpe (#"},
		{mas1, "debito masivo sinpe (#"},
		{cta1, "credito en cuenta #"},
		{"", ""},
	}
	for _, c := range casos {
		if got := formaDescripcion(c.desc); got != c.quiere {
			t.Errorf("formaDescripcion(%.30q) = %q, quiere %q", c.desc, got, c.quiere)
		}
	}
}

func TestFormaSeparaSinpeEntranteDeSaliente(t *testing.T) {
	// El caso que motivó los 4 tokens: con 3 se fundirían el cobro a clientes (crédito) y el
	// pago a proveedores (débito), que son hechos opuestos.
	if formaDescripcion(smo1) == formaDescripcion(sal1) {
		t.Fatalf("SINPE Móvil entrante y SINPE saliente comparten forma: %q", formaDescripcion(smo1))
	}
}

func TestPrefijoComunYRecorte(t *testing.T) {
	if got := prefijoComun([]string{"abcdef", "abcxyz", "abc"}); got != "abc" {
		t.Errorf("prefijoComun = %q, quiere abc", got)
	}
	if got := prefijoComun(nil); got != "" {
		t.Errorf("prefijoComun(nil) = %q", got)
	}
	if got := prefijoComun([]string{"xyz", "abc"}); got != "" {
		t.Errorf("sin prefijo común debería dar vacío, dio %q", got)
	}
	if got := recortarAntesDeDigitos("dr/cr linea sinpe (smo-2026"); got != "dr/cr linea sinpe (smo-" {
		t.Errorf("recorte = %q", got)
	}
	if got := recortarAntesDeDigitos("sin digitos"); got != "sin digitos" {
		t.Errorf("sin dígitos no debería recortar, dio %q", got)
	}
}

func TestAgruparPatronesCasoReal(t *testing.T) {
	lineas := []LineaSinClasificar{}
	lineas = append(lineas, repetir(credito(smo1), 12)...)
	lineas = append(lineas, credito(smo2), credito(smo3))
	lineas = append(lineas, repetir(debito(sal1), 6)...)
	lineas = append(lineas, debito(sal2))
	lineas = append(lineas, repetir(debito(mas1), 5)...)
	lineas = append(lineas, debito(mas2))
	lineas = append(lineas, repetir(credito(cta1), 5)...)
	lineas = append(lineas, credito(cta2))
	// Un hecho aislado no debe proponerse como regla.
	lineas = append(lineas, debito("PAGO UNICO DE ALGO RARO 998877"))

	todas := make([]string, 0, len(lineas))
	for _, l := range lineas {
		todas = append(todas, l.Descripcion)
	}

	pats := AgruparPatrones(lineas, todas, 25)
	if len(pats) != 4 {
		t.Fatalf("grupos = %d, quiere 4 (SMO, SINPE saliente, masivo, crédito en cuenta): %+v", len(pats), pats)
	}

	t.Run("el grupo más grande va primero y es SINPE Móvil de clientes", func(t *testing.T) {
		p := pats[0]
		if p.Movimientos != 14 || p.Creditos != 14 || p.Debitos != 0 {
			t.Errorf("grupo 1: %d movs (%d cr / %d db)", p.Movimientos, p.Creditos, p.Debitos)
		}
		if p.AplicaA != "CREDITO" {
			t.Errorf("aplica_a = %s, quiere CREDITO (son todos créditos)", p.AplicaA)
		}
		if !strings.Contains(p.Patron, "smo-") {
			t.Errorf("el patrón debería identificar SMO, es %q", p.Patron)
		}
	})

	t.Run("no propone una palabra que arrastre un consecutivo", func(t *testing.T) {
		// El SINPE saliente solo comparte la referencia de 25 dígitos: una regla con eso
		// clasificaría los de hoy y nunca más. Se reporta el grupo, sin palabra.
		var saliente *PatronSugerido
		for i := range pats {
			if pats[i].Movimientos == 7 {
				saliente = &pats[i]
			}
		}
		if saliente == nil {
			t.Fatalf("falta el grupo de SINPE saliente: %+v", pats)
		}
		if saliente.Patron != "" || saliente.Motivo != "SOLO_REFERENCIAS" {
			t.Errorf("saliente: patrón %q, motivo %q (quiere vacío + SOLO_REFERENCIAS)",
				saliente.Patron, saliente.Motivo)
		}
		if saliente.Alcance != 0 {
			t.Errorf("sin palabra no hay alcance que medir, dio %d", saliente.Alcance)
		}
	})

	t.Run("los patrones propuestos sobreviven al cambio de año", func(t *testing.T) {
		for _, p := range pats {
			if p.Patron != "" && p.AvisoAnio {
				t.Errorf("patrón %q arrastra un año sin necesidad", p.Patron)
			}
		}
	})

	t.Run("ningún patrón propuesto invade otra forma", func(t *testing.T) {
		for _, p := range pats {
			if p.Patron == "" {
				continue
			}
			if p.Ajenos != 0 {
				t.Errorf("patrón %q toca %d movimientos de otra forma", p.Patron, p.Ajenos)
			}
		}
	})

	t.Run("los montos se suman por grupo", func(t *testing.T) {
		if pats[0].Monto != "140000.00" {
			t.Errorf("monto del grupo SMO = %s, quiere 140000.00 (14 × 10 000)", pats[0].Monto)
		}
	})

	t.Run("trae ejemplos reconocibles y no más de dos", func(t *testing.T) {
		for _, p := range pats {
			if len(p.Ejemplos) == 0 || len(p.Ejemplos) > 2 {
				t.Errorf("patrón %q: %d ejemplos", p.Patron, len(p.Ejemplos))
			}
		}
	})
}

func TestAgruparPatronesAvisaAnioCuandoEsInevitable(t *testing.T) {
	// Grupo cuyo prefijo sin el año («planilla») invadiría OTRO hecho: hay que conservar el
	// año, y entonces se avisa que la regla dejará de calzar cuando cambie.
	lineas := []LineaSinClasificar{}
	for i := 0; i < 6; i++ {
		lineas = append(lineas, debito("PLANILLA 2026 QUINCENA UNO REF "+string(rune('A'+i))))
	}
	otras := []string{"PLANILLA ADELANTOS ESPECIAL DIRECCION"}
	for _, l := range lineas {
		otras = append(otras, l.Descripcion)
	}
	pats := AgruparPatrones(lineas, otras, 10)
	if len(pats) != 1 {
		t.Fatalf("grupos = %d, quiere 1: %+v", len(pats), pats)
	}
	p := pats[0]
	if p.Patron != "planilla 2026 quincena uno ref" {
		t.Errorf("patrón = %q, quiere «planilla 2026 quincena uno ref» (sin el año invadiría los adelantos)", p.Patron)
	}
	if !p.AvisoAnio {
		t.Errorf("el patrón %q contiene el año y debería avisarlo", p.Patron)
	}
	if p.Alterna != "planilla" {
		t.Errorf("debería ofrecer «planilla» como alternativa, ofreció %q", p.Alterna)
	}
	if p.Ajenos != 0 {
		t.Errorf("el patrón elegido no debería tocar otras formas, toca %d", p.Ajenos)
	}
}

func TestAgruparPatronesNoInvadeOtroHecho(t *testing.T) {
	// Una palabra que calza con movimientos ya clasificados de la MISMA forma es correcta:
	// es el mismo hecho, clasificado antes a mano. Eso no la descalifica.
	lineas := []LineaSinClasificar{}
	for i := 0; i < 6; i++ {
		lineas = append(lineas, credito("NC DEPOSITO TUCAN CC // CUOTA "+string(rune('0'+i))))
	}
	todas := []string{}
	for _, l := range lineas {
		todas = append(todas, l.Descripcion)
	}
	// 20 más de la misma forma, ya clasificados (no vienen en sinClasificar).
	for i := 0; i < 20; i++ {
		todas = append(todas, "NC DEPOSITO TUCAN CC // CUOTA YA CLASIFICADA")
	}
	pats := AgruparPatrones(lineas, todas, 10)
	if len(pats) != 1 {
		t.Fatalf("grupos = %d, quiere 1", len(pats))
	}
	if pats[0].Patron == "" {
		t.Fatalf("debería proponer palabra: %+v", pats[0])
	}
	if pats[0].Ajenos != 0 {
		t.Errorf("los ya clasificados son de la misma forma: ajenos debería ser 0, dio %d", pats[0].Ajenos)
	}
	if pats[0].Alcance != 26 {
		t.Errorf("alcance = %d, quiere 26 (6 pendientes + 20 ya clasificados del mismo hecho)", pats[0].Alcance)
	}
}

func TestAgruparPatronesIgnoraGruposChicosYVacios(t *testing.T) {
	lineas := []LineaSinClasificar{debito("ALGO 1"), debito("ALGO 2"), credito("OTRA COSA 3")}
	if pats := AgruparPatrones(lineas, []string{}, 10); len(pats) != 0 {
		t.Errorf("con grupos de 2 y 1 no debería proponer nada, propuso %+v", pats)
	}
	if pats := AgruparPatrones(nil, nil, 10); len(pats) != 0 {
		t.Errorf("sin líneas no hay patrones, dio %+v", pats)
	}
}

func TestAgruparPatronesGrupoMixtoNoAdivina(t *testing.T) {
	lineas := []LineaSinClasificar{}
	for i := 0; i < 4; i++ {
		lineas = append(lineas, credito("TRANSFERENC BANCOBCR / REF 100"+string(rune('0'+i))))
		lineas = append(lineas, debito("TRANSFERENC BANCOBCR / REF 200"+string(rune('0'+i))))
	}
	pats := AgruparPatrones(lineas, nil, 10)
	if len(pats) != 1 {
		t.Fatalf("grupos = %d, quiere 1", len(pats))
	}
	if pats[0].AplicaA != "MIXTO" {
		t.Errorf("aplica_a = %s: con créditos y débitos mezclados no se adivina el signo", pats[0].AplicaA)
	}
	if pats[0].Creditos != 4 || pats[0].Debitos != 4 {
		t.Errorf("desglose de signos: %d cr / %d db", pats[0].Creditos, pats[0].Debitos)
	}
}

func TestAgruparPatronesLimite(t *testing.T) {
	lineas := []LineaSinClasificar{}
	for g := 0; g < 5; g++ {
		for i := 0; i < 6; i++ {
			lineas = append(lineas, debito("GRUPO"+string(rune('A'+g))+" MOVIMIENTO "+string(rune('0'+i))))
		}
	}
	if pats := AgruparPatrones(lineas, nil, 2); len(pats) != 2 {
		t.Errorf("con límite 2 debería devolver 2, dio %d", len(pats))
	}
}
