package cxp

// Que las PRÓXIMAS facturas de un proveedor no nazcan bloqueadas (4-nov-2026).
//
// EL PEDIDO, textual del Director Financiero: «cuando una factura ya fue liberada, debería
// guardar el comportamiento de que las próximas deben venir liberadas; de lo contrario siempre
// hay que irlas a liberar y ya es algo de la cotidianidad del proveedor».
//
// Lo que se prueba acá es CUÁNDO se toca la ficha y cuándo no. Tocarla de más cambia cómo nacen
// TODAS las facturas futuras de ese proveedor por una operación que ni siquiera funcionó.

import (
	"context"
	"strings"
	"testing"

	"go.uber.org/zap"
)

const motivoReal = "El proveedor factura contado pero tenemos convenio a 30 días"

func svcConFake(f *fakeRepo) *Service { return NewService(f, nil, zap.NewNop()) }

// EL CASO: se liberan facturas y la ficha queda lista para que las próximas no nazcan trancadas.
func TestAlLiberarSeDejaLaFichaParaLasProximas(t *testing.T) {
	f := &fakeRepo{
		liberadas:          []FacturaLiberada{{DocumentoID: "d1", Consecutivo: "001"}},
		configuroProveedor: true,
	}
	res, err := svcConFake(f).LiberarParaPago(context.Background(), "emp", "prov-1",
		LiberacionInput{DocumentoIDs: []string{"d1"}, Motivo: motivoReal, ConfigurarProveedor: true}, "usr")
	if err != nil {
		t.Fatalf("liberar: %v", err)
	}
	if !f.seConfiguroProveedor {
		t.Fatal("no se tocó la ficha: las próximas volverían a nacer bloqueadas")
	}
	if !res.ProveedorConfigurado {
		t.Error("no se informó que la ficha quedó configurada")
	}
	// EL CAMBIO SE ANUNCIA. Gobierna todas las facturas futuras del proveedor, no solo las que se
	// acaban de liberar: hacerlo en silencio es lo que después nadie puede explicar.
	if !strings.Contains(res.QueSeConfiguro, "no van a nacer bloqueadas") {
		t.Errorf("el mensaje no dice qué quedó configurado: %q", res.QueSeConfiguro)
	}
}

// SI NO SE LIBERÓ NADA, LA FICHA NO SE TOCA.
//
// Configurar al proveedor por una operación que falló lo dejaría cambiado para siempre por algo
// que no pasó, y nadie ataría una cosa con la otra después.
func TestSiNoSeLiberoNadaLaFichaNoSeToca(t *testing.T) {
	f := &fakeRepo{
		liberadas:     nil, // el UPDATE no alcanzó ninguna fila
		estadoBloqueo: map[string]bool{"d1": false},
	}
	res, err := svcConFake(f).LiberarParaPago(context.Background(), "emp", "prov-1",
		LiberacionInput{DocumentoIDs: []string{"d1"}, Motivo: motivoReal, ConfigurarProveedor: true}, "usr")
	if err != nil {
		t.Fatalf("liberar: %v", err)
	}
	if f.seConfiguroProveedor {
		t.Error("se configuró la ficha sin haber liberado nada")
	}
	if res.ProveedorConfigurado {
		t.Error("se informó una configuración que no ocurrió")
	}
}

// Sin pedirlo, la ficha tampoco se toca: la casilla viene marcada en la pantalla, pero quien la
// desmarca está diciendo «esto es una excepción» y hay que respetarlo.
func TestSinPedirloLaFichaNoSeToca(t *testing.T) {
	f := &fakeRepo{liberadas: []FacturaLiberada{{DocumentoID: "d1"}}}
	res, err := svcConFake(f).LiberarParaPago(context.Background(), "emp", "prov-1",
		LiberacionInput{DocumentoIDs: []string{"d1"}, Motivo: motivoReal, ConfigurarProveedor: false}, "usr")
	if err != nil {
		t.Fatalf("liberar: %v", err)
	}
	if f.seConfiguroProveedor || res.ProveedorConfigurado {
		t.Error("se configuró la ficha sin que nadie lo pidiera")
	}
}

// Si la ficha YA estaba configurada, no se anuncia un cambio que no ocurrió.
func TestSiLaFichaYaEstabaNoSeAnunciaUnCambio(t *testing.T) {
	f := &fakeRepo{
		liberadas:          []FacturaLiberada{{DocumentoID: "d1"}},
		configuroProveedor: false, // el UPDATE no cambió nada: ya estaba así
	}
	res, err := svcConFake(f).LiberarParaPago(context.Background(), "emp", "prov-1",
		LiberacionInput{DocumentoIDs: []string{"d1"}, Motivo: motivoReal, ConfigurarProveedor: true}, "usr")
	if err != nil {
		t.Fatalf("liberar: %v", err)
	}
	if !strings.Contains(res.QueSeConfiguro, "ya estaba configurada") {
		t.Errorf("anunció un cambio que no ocurrió: %q", res.QueSeConfiguro)
	}
}

// El motivo sigue siendo obligatorio: la casilla nueva no relaja la única barrera contra el
// doble pago.
func TestLaCasillaNoRelajaElMotivoObligatorio(t *testing.T) {
	f := &fakeRepo{liberadas: []FacturaLiberada{{DocumentoID: "d1"}}}
	_, err := svcConFake(f).LiberarParaPago(context.Background(), "emp", "prov-1",
		LiberacionInput{DocumentoIDs: []string{"d1"}, Motivo: "ok", ConfigurarProveedor: true}, "usr")
	if err != ErrMotivoLiberacionRequerido {
		t.Errorf("se liberó con un motivo de dos letras: %v", err)
	}
	if f.seConfiguroProveedor {
		t.Error("se tocó la ficha pese a que la liberación no pasó la validación")
	}
}
