package conta

// Importar el mapeo desde el .xlsx, contra PostgreSQL de verdad (12-nov-2026).
//
// EL DEFECTO QUE ESTO EVITA NO DA NINGÚN ERROR. El archivo trae nombres que alguien editó y
// códigos de cuenta tecleados; si la importación aplicara «la partida más parecida» o «la cuenta
// que suene igual», el mapeo quedaría puesto en la partida equivocada, el sistema diría «12
// aplicadas» y el problema saldría a la luz meses después, en un asiento que nadie entiende.
//
// Por eso las tres pruebas de acá miran lo mismo desde tres lados: que lo que no calza EXACTO se
// rechace nombrando la fila del Excel, y que lo que sí calza entre aunque haya rechazos al lado.

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
	"go.uber.org/zap"
)

// hojaDePrueba arma un .xlsx con el encabezado real y las filas que se le pasen.
func hojaDePrueba(t *testing.T, filas [][]string) []byte {
	t.Helper()
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	if err := f.SetSheetName("Sheet1", hojaMapeo); err != nil {
		t.Fatalf("nombrar hoja: %v", err)
	}
	todas := append([][]string{
		{"Concepto", "Partida", "Código de cuenta", "Motivo si no se contabiliza"},
	}, filas...)
	for i, fila := range todas {
		for j, v := range fila {
			celda, _ := excelize.CoordinatesToCellName(j+1, i+1)
			if err := f.SetCellValue(hojaMapeo, celda, v); err != nil {
				t.Fatalf("escribir celda: %v", err)
			}
		}
	}
	buf, err := f.WriteToBuffer()
	if err != nil {
		t.Fatalf("serializar: %v", err)
	}
	return buf.Bytes()
}

// rechazoDe busca el rechazo de una fila por el nombre de la partida.
func rechazoDe(res ResultadoImportacionMapeo, partida string) (RechazoDeFila, bool) {
	for _, r := range res.Rechazos {
		if r.Clasificacion == partida {
			return r, true
		}
	}
	return RechazoDeFila{}, false
}

// EL CAMINO FELIZ, y que una fila mala no tumbe a las buenas.
func TestImportarElMapeoAplicaLoBuenoYNombraLoMalo(t *testing.T) {
	repo := baseMapeo(t)
	svc := NewService(repo, nil, zap.NewNop())
	ctx := context.Background()

	sembrarPartida(t, repo, empUno, "Gastos", NatGasto, true, "Combustible", 3)
	sembrarPartida(t, repo, empUno, "Control", NatNeutro, true, "Traslado de Fondos", 2)
	if _, err := repo.CrearCuenta(ctx,
		cuenta("6-01-01-030-000", "Combustible", idDe(empUno), true, ""), usrPrueba); err != nil {
		t.Fatalf("sembrar cuenta: %v", err)
	}

	archivo := hojaDePrueba(t, [][]string{
		{"Gastos", "Combustible", "6-01-01-030-000", ""},
		{"Control", "Traslado de Fondos", "", "es movimiento entre cuentas propias de la empresa"},
		{"Gastos", "Partida Que No Existe", "6-01-01-030-000", ""},
		{"Gastos", "Combustible", "9-99-99-999-999", ""},
	})

	res, err := svc.ImportarMapeoXLSX(ctx, empUno, archivo, usrPrueba)
	if err != nil {
		t.Fatalf("importar: %v", err)
	}

	if res.Aplicadas != 2 {
		t.Errorf("aplicadas = %d, se esperaban 2 (combustible y traslado)", res.Aplicadas)
	}
	if res.Resumen.FilasLeidas != 4 {
		t.Errorf("filas leídas = %d, se esperaban 4", res.Resumen.FilasLeidas)
	}
	if len(res.Rechazos) != 2 {
		t.Fatalf("rechazos = %d (%+v), se esperaban 2", len(res.Rechazos), res.Rechazos)
	}

	// LA PARTIDA INVENTADA SE RECHAZA, no se aplica a la más parecida.
	r, ok := rechazoDe(res, "Partida Que No Existe")
	if !ok {
		t.Fatal("no se rechazó la partida inexistente: se aplicó a alguna otra")
	}
	if r.Fila != 4 {
		t.Errorf("el rechazo dice fila %d; en el Excel es la 4", r.Fila)
	}
	if !strings.Contains(r.Motivo, "no existe") {
		t.Errorf("motivo = %q; tiene que decir que la partida no existe", r.Motivo)
	}

	// LA CUENTA INVENTADA TAMBIÉN, y el motivo nombra el código para poder corregirlo.
	r2, ok := rechazoDe(res, "Combustible")
	if !ok {
		t.Fatal("no se rechazó la fila con la cuenta inexistente")
	}
	if !strings.Contains(r2.Motivo, "9-99-99-999-999") {
		t.Errorf("motivo = %q; tiene que nombrar el código que no existe", r2.Motivo)
	}
}

// Una cuenta de AGRUPACIÓN se rechaza con su nombre, no con un error genérico de la base.
func TestImportarRechazaLaCuentaDeAgrupacionNombrandola(t *testing.T) {
	repo := baseMapeo(t)
	svc := NewService(repo, nil, zap.NewNop())
	ctx := context.Background()

	sembrarPartida(t, repo, empUno, "Gastos", NatGasto, true, "Combustible", 1)
	if _, err := repo.CrearCuenta(ctx,
		cuenta("6-01-01-000-000", "Gastos de Operación", idDe(empUno), false, ""), usrPrueba); err != nil {
		t.Fatalf("sembrar la madre: %v", err)
	}
	if _, err := repo.CrearCuenta(ctx,
		cuenta("6-01-01-030-000", "Combustible", idDe(empUno), true, "6-01-01-000-000"), usrPrueba); err != nil {
		t.Fatalf("sembrar la hija: %v", err)
	}

	archivo := hojaDePrueba(t, [][]string{
		{"Gastos", "Combustible", "6-01-01-000-000", ""},
	})
	res, err := svc.ImportarMapeoXLSX(ctx, empUno, archivo, usrPrueba)
	if err != nil {
		t.Fatalf("importar: %v", err)
	}
	if res.Aplicadas != 0 {
		t.Errorf("aplicadas = %d; una cuenta de agrupación no admite asientos", res.Aplicadas)
	}
	r, ok := rechazoDe(res, "Combustible")
	if !ok {
		t.Fatal("no hubo rechazo")
	}
	if !strings.Contains(r.Motivo, "Gastos de Operación") {
		t.Errorf("motivo = %q; tiene que nombrar la cuenta para poder corregirla", r.Motivo)
	}
}

// El archivo de OTRA empresa no se aplica acá: se rechaza entero, nombrando cada fila.
func TestImportarNoAplicaElArchivoDeOtraEmpresa(t *testing.T) {
	repo := baseMapeo(t)
	svc := NewService(repo, nil, zap.NewNop())
	ctx := context.Background()

	// La partida vive en la empresa DOS; la importación corre sobre la UNO.
	sembrarPartida(t, repo, empDos, "Gastos", NatGasto, true, "Combustible de Memorial", 2)
	if _, err := repo.CrearCuenta(ctx,
		cuenta("6-01-01-030-000", "Combustible", idDe(empUno), true, ""), usrPrueba); err != nil {
		t.Fatalf("sembrar cuenta: %v", err)
	}

	archivo := hojaDePrueba(t, [][]string{
		{"Gastos", "Combustible de Memorial", "6-01-01-030-000", ""},
	})
	res, err := svc.ImportarMapeoXLSX(ctx, empUno, archivo, usrPrueba)
	if err != nil {
		t.Fatalf("importar: %v", err)
	}
	if res.Aplicadas != 0 {
		t.Errorf("aplicadas = %d; la partida es de otra empresa", res.Aplicadas)
	}
	if _, ok := rechazoDe(res, "Combustible de Memorial"); !ok {
		t.Error("no se rechazó la fila de la otra empresa")
	}
}

// Un archivo ilegible no pasa por el camino de importación.
func TestImportarRechazaLoQueNoEsExcel(t *testing.T) {
	repo := baseMapeo(t)
	svc := NewService(repo, nil, zap.NewNop())
	_, err := svc.ImportarMapeoXLSX(context.Background(), empUno,
		[]byte("esto no es un xlsx"), usrPrueba)
	if err != ErrMapeoArchivoIlegible {
		t.Errorf("err = %v, se esperaba ErrMapeoArchivoIlegible", err)
	}
}

// La exportación trae lo que hay y se puede volver a leer: la vuelta completa.
func TestLoExportadoSePuedeVolverALeer(t *testing.T) {
	repo := baseMapeo(t)
	svc := NewService(repo, nil, zap.NewNop())
	ctx := context.Background()

	clasifID := sembrarPartida(t, repo, empUno, "Gastos", NatGasto, true, "Combustible", 3)
	conMonto(t, repo, clasifID, "1000")
	if _, err := repo.CrearCuenta(ctx,
		cuenta("6-01-01-030-000", "Combustible", idDe(empUno), true, ""), usrPrueba); err != nil {
		t.Fatalf("sembrar cuenta: %v", err)
	}

	datos, err := svc.ExportarMapeoXLSX(ctx, empUno)
	if err != nil {
		t.Fatalf("exportar: %v", err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(datos))
	if err != nil {
		t.Fatalf("abrir lo exportado: %v", err)
	}
	defer func() { _ = f.Close() }()

	filas, err := f.GetRows(hojaMapeo)
	if err != nil || len(filas) < 2 {
		t.Fatalf("lo exportado no trae filas: %v", err)
	}
	if filas[1][1] != "Combustible" {
		t.Errorf("la primera partida exportada es %q", filas[1][1])
	}
	// Y el plan viaja en su hoja, que es lo que permite llenar el archivo sin tenerlo abierto aparte.
	plan, err := f.GetRows(hojaPlan)
	if err != nil || len(plan) < 2 {
		t.Fatalf("lo exportado no trae el plan: %v", err)
	}
	if plan[1][0] != "6-01-01-030-000" {
		t.Errorf("el plan exportado trae %q", plan[1][0])
	}
}
