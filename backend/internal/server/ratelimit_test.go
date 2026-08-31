package server

import (
	"testing"
	"time"
)

func TestRateLimiterVentana(t *testing.T) {
	rl := newRateLimiter(3, time.Minute)
	base := time.Unix(1_000_000, 0)
	rl.now = func() time.Time { return base }

	// Los primeros 3 golpes pasan.
	for i := 1; i <= 3; i++ {
		if ok, _ := rl.permitir("1.2.3.4"); !ok {
			t.Fatalf("golpe %d: debería permitir", i)
		}
	}
	// El 4.º se rechaza con un Retry-After > 0.
	ok, retry := rl.permitir("1.2.3.4")
	if ok {
		t.Fatal("el 4.º golpe debería rechazarse")
	}
	if retry <= 0 {
		t.Errorf("Retry-After = %d, quería > 0", retry)
	}

	// Otra IP no se ve afectada.
	if ok, _ := rl.permitir("9.9.9.9"); !ok {
		t.Error("otra IP debería tener su propio cupo")
	}

	// Al pasar la ventana, el cupo se libera.
	rl.now = func() time.Time { return base.Add(time.Minute + time.Second) }
	if ok, _ := rl.permitir("1.2.3.4"); !ok {
		t.Error("tras la ventana debería volver a permitir")
	}
}

func TestRateLimiterLimpiar(t *testing.T) {
	rl := newRateLimiter(2, time.Minute)
	base := time.Unix(2_000_000, 0)
	rl.now = func() time.Time { return base }
	rl.permitir("a")
	rl.permitir("b")

	// Avanza más allá de la ventana y limpia: el mapa queda vacío.
	rl.now = func() time.Time { return base.Add(2 * time.Minute) }
	rl.limpiar()
	rl.mu.Lock()
	n := len(rl.hits)
	rl.mu.Unlock()
	if n != 0 {
		t.Errorf("tras limpiar debería quedar 0 claves, quedan %d", n)
	}
}
