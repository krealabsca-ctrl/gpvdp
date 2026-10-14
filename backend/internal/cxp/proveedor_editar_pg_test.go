package cxp

// Editar un proveedor, contra PostgreSQL de verdad (12-oct-2026).
//
// POR QUÉ EXISTE ESTE ARCHIVO. La edición de proveedores estuvo ROTA ENTERA desde la migración
// 0095: el UPDATE declaraba `convenio_manda = COALESCE(NULLIF($17,''), convenio_manda)` y la lista de
// argumentos terminaba en `$16`. pgx rechaza eso antes de tocar la base —«expected 17 arguments,
// got 16»— así que no fallaba solo el campo nuevo: **no se podía guardar ningún cambio de ningún
// proveedor**, ni el nombre, ni el IBAN, ni el correo.
//
// No lo encontró ninguna prueba porque no había NINGUNA sobre `Actualizar`. Lo encontró el
// Director Financiero, con un «error interno» en pantalla que no decía nada.
//
// La lección, que es vieja en este proyecto: un repositorio probado solo detrás de un fake no
// está probado. Un fake de `Actualizar` habría devuelto el proveedor feliz sin mirar un solo
// placeholder.

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"
)

const provEditar = "b1b1b1b1-b1b1-4b1b-9b1b-b1b1b1b1b1b1"

func baseDeProveedor(t *testing.T) *pgRepository {
	t.Helper()
	repo := baseDeLotes(t)
	if _, err := repo.pool.Exec(context.Background(),
		`INSERT INTO proveedor (id, empresa_id, nombre, identificacion, condicion_pago, plazo_credito_dias)
		 VALUES ($1::uuid, $2::uuid, 'KEISY MELISSA CECILIANO MATA', '116290134', 'CONTADO', 0)`,
		provEditar, empresaLote); err != nil {
		t.Fatalf("sembrar proveedor: %v", err)
	}
	return repo
}

// EL CASO EXACTO QUE FALLÓ: pasar un proveedor de contado a crédito, con IBAN y correo.
func TestEditarProveedorGuardaTodosLosCampos(t *testing.T) {
	repo := baseDeProveedor(t)
	ctx := context.Background()

	p, err := repo.Actualizar(ctx, empresaLote, provEditar, ProveedorInput{
		Nombre:             "Keisy Melissa Ceciliano Mata",
		TipoIdentificacion: "FISICA",
		Identificacion:     "116290134",
		Email:              "cecilianokeisy@gmail.com",
		Telefono:           "8529-9792",
		IBAN:               "CR16015202001239892030",
		RetencionRentaPct:  decimal.Zero,
		CondicionPago:      "CREDITO",
		PlazoCreditoDias:   8,
		Departamento:       "Logística",
	})
	if err != nil {
		t.Fatalf("actualizar: %v", err)
	}

	if p.Nombre != "Keisy Melissa Ceciliano Mata" {
		t.Errorf("nombre = %q", p.Nombre)
	}
	if p.CondicionPago != "CREDITO" {
		t.Errorf("condición = %q, quería CREDITO — sin esto la factura no se puede pagar por banco", p.CondicionPago)
	}
	if p.PlazoCreditoDias != 8 {
		t.Errorf("plazo = %d, quería 8", p.PlazoCreditoDias)
	}
	if p.IBAN != "CR16015202001239892030" {
		t.Errorf("IBAN = %q", p.IBAN)
	}
	if p.Email != "cecilianokeisy@gmail.com" {
		t.Errorf("email = %q", p.Email)
	}
}

// EL CAMPO VACÍO CONSERVA LO CONFIGURADO (regla de la mig 0095).
//
// Una edición que no toca «el vencimiento lo manda» no puede devolver el proveedor al plazo del
// XML sin que nadie lo pida: eso le movería la fecha de vencimiento a todas sus facturas futuras.
func TestEditarProveedorNoPierdeElConvenioManda(t *testing.T) {
	repo := baseDeProveedor(t)
	ctx := context.Background()

	base := ProveedorInput{Nombre: "Keisy", CondicionPago: "CREDITO", PlazoCreditoDias: 8}

	// Se configura que mande el convenio…
	conConvenio := base
	conConvenio.ConvenioManda = "PROVEEDOR"
	if _, err := repo.Actualizar(ctx, empresaLote, provEditar, conConvenio); err != nil {
		t.Fatalf("configurar convenio_manda: %v", err)
	}

	// …y después se edita otra cosa sin tocar ese campo.
	soloNombre := base
	soloNombre.Nombre = "Keisy Melissa Ceciliano Mata"
	p, err := repo.Actualizar(ctx, empresaLote, provEditar, soloNombre)
	if err != nil {
		t.Fatalf("segunda edición: %v", err)
	}
	if p.ConvenioManda != "PROVEEDOR" {
		t.Errorf("convenio_manda = %q tras una edición que no lo tocaba, quería PROVEEDOR", p.ConvenioManda)
	}

	// Y cuando SÍ se cambia, cambia.
	vuelveAlXML := base
	vuelveAlXML.ConvenioManda = "XML"
	p, err = repo.Actualizar(ctx, empresaLote, provEditar, vuelveAlXML)
	if err != nil {
		t.Fatalf("volver al XML: %v", err)
	}
	if p.ConvenioManda != "XML" {
		t.Errorf("convenio_manda = %q, quería XML", p.ConvenioManda)
	}
}

// Un proveedor de OTRA empresa no se edita aunque se pase su id.
func TestEditarProveedorNoCruzaEmpresas(t *testing.T) {
	repo := baseDeProveedor(t)
	_, err := repo.Actualizar(context.Background(), otraEmpLote, provEditar,
		ProveedorInput{Nombre: "Robado"})
	if err == nil {
		t.Fatal("se editó un proveedor de otra empresa")
	}
	if err != ErrProveedorNoEncontrado {
		t.Errorf("error = %v, quería ErrProveedorNoEncontrado", err)
	}
}
