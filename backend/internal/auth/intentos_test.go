package auth

// El límite de intentos de ingreso (21-oct-2026). El porqué está en intentos.go.
//
// El tiempo se inyecta en vez de dormir: un test que espera quince minutos no es un test.

import (
	"testing"
	"time"
)

// reloj devuelve un contador con el tiempo bajo control.
func reloj(t *testing.T) (*Intentos, func(time.Duration)) {
	t.Helper()
	ahora := time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC)
	i := NuevoIntentos()
	i.ahora = func() time.Time { return ahora }
	return i, func(d time.Duration) { ahora = ahora.Add(d) }
}

// LA PROPIEDAD QUE MÁS IMPORTA: no se puede dejar afuera a una persona de verdad.
//
// Si el contador fuera por CORREO, a cualquiera le bastaría escribir mal la contraseña de la
// tesorera cinco veces para dejarla sin entrar el día del pago de planilla — el control contra la
// fuerza bruta convertido en una herramienta de denegación de servicio contra quien trabaja.
//
// Falsificación: cambiar la clave de `claveCorreoOrigen` para que ignore el origen hace que este
// test falle diciendo exactamente eso.
func TestUnAtacanteNoPuedeDejarAfueraAlUsuarioReal(t *testing.T) {
	i, _ := reloj(t)
	const correo = "tesoreria@valledepaz.cr"

	// El atacante quema sus intentos desde su IP.
	for n := 0; n < MaxPorCorreoYOrigen; n++ {
		i.Fallo(correo, "203.0.113.50")
	}
	if bloqueado, _ := i.Bloqueado(correo, "203.0.113.50"); !bloqueado {
		t.Fatal("el atacante siguió pudiendo intentar después de agotar el límite")
	}

	// La persona de verdad, desde la oficina, entra sin enterarse.
	if bloqueado, _ := i.Bloqueado(correo, "190.10.1.7"); bloqueado {
		t.Error("se dejó afuera al usuario REAL desde otro equipo: cualquiera podría bloquear a la tesorera escribiendo mal su contraseña cinco veces")
	}
}

// Y el que rota correos tampoco pasa: el segundo contador es por origen.
func TestRotarCorreosNoEsquivaElLimite(t *testing.T) {
	i, _ := reloj(t)
	const origen = "203.0.113.50"

	// Un correo distinto en cada intento: la clave de la pareja nunca llega a cinco.
	for n := 0; n < MaxPorOrigen; n++ {
		i.Fallo(string(rune('a'+n%26))+"@prueba.local", origen)
	}
	if bloqueado, _ := i.Bloqueado("nuevo@prueba.local", origen); !bloqueado {
		t.Error("rotando correos se pudo seguir probando desde el mismo origen sin tope")
	}
}

// Los fallos caducan: quien se equivocó puede reintentar sin llamar a nadie.
func TestLosFallosCaducan(t *testing.T) {
	i, avanzar := reloj(t)
	const correo, origen = "alguien@prueba.local", "190.10.1.7"

	for n := 0; n < MaxPorCorreoYOrigen; n++ {
		i.Fallo(correo, origen)
	}
	if bloqueado, falta := i.Bloqueado(correo, origen); !bloqueado || falta <= 0 {
		t.Fatalf("esperaba bloqueo con espera positiva (bloqueado=%v falta=%v)", bloqueado, falta)
	}

	avanzar(VentanaIntentos + time.Second)
	if bloqueado, _ := i.Bloqueado(correo, origen); bloqueado {
		t.Error("el bloqueo no caducó: quien se equivocó quedaría afuera para siempre")
	}
}

// Entrar bien limpia la pareja, pero NO el contador del origen.
//
// Si lo limpiara, quien tiene UNA credencial válida podría entrar, borrar el contador del lugar y
// seguir probando otras cuentas a voluntad — justo lo que el segundo contador existe para impedir.
func TestEntrarBienNoLimpiaElContadorDelOrigen(t *testing.T) {
	i, _ := reloj(t)
	const origen = "203.0.113.50"

	for n := 0; n < MaxPorOrigen; n++ {
		i.Fallo(string(rune('a'+n%26))+"@prueba.local", origen)
	}
	// Entra con una cuenta buena desde el mismo lugar.
	i.Exito("bueno@prueba.local", origen)

	if bloqueado, _ := i.Bloqueado("otro@prueba.local", origen); !bloqueado {
		t.Error("entrar con una cuenta válida borró el tope del origen: con una credencial buena se podría probar el resto sin límite")
	}
}

// Entrar bien sí limpia la pareja: dos errores de dedo y después la correcta no dejan rastro.
func TestEntrarBienLimpiaLaPareja(t *testing.T) {
	i, _ := reloj(t)
	const correo, origen = "alguien@prueba.local", "190.10.1.7"

	i.Fallo(correo, origen)
	i.Fallo(correo, origen)
	i.Exito(correo, origen)

	// Después del acierto, vuelve a tener los cinco intentos completos.
	for n := 0; n < MaxPorCorreoYOrigen-1; n++ {
		i.Fallo(correo, origen)
	}
	if bloqueado, _ := i.Bloqueado(correo, origen); bloqueado {
		t.Error("los fallos anteriores al ingreso correcto siguieron contando")
	}
}
