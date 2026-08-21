// Package seed siembra los datos base del ERP: roles, 3 empresas, catálogo demo y admin.
// Es idempotente (se puede correr varias veces sin duplicar).
package seed

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/gpvdp/erp/internal/auth"
	"github.com/gpvdp/erp/internal/config"
)

var roles = []struct{ Codigo, Nombre, Desc string }{
	{"ADMIN", "Administrador", "Acceso total (configuración inicial)"},
	{"DIRECTOR_FINANCIERO", "Director Financiero", "Dashboard, proyecciones, congelar TC"},
	{"SUPERVISOR_FINANCIERO", "Supervisor Financiero", "Valida clasificación y aprueba reglas"},
	{"AUXILIAR_FINANCIERO", "Auxiliar Financiero", "Importa, clasifica y cuadra"},
	{"GERENCIA_GENERAL", "Gerencia General", "Vista consolidada de lectura"},
	{"AUDITOR_INTERNO", "Auditor Interno", "Solo lectura y trazabilidad"},
}

var empresas = []string{"Valle de Paz", "Coopeprofa", "Memorial Pets"}

// Catálogo mínimo de demostración por empresa (Concepto -> Clasificaciones).
var conceptosDemo = []struct {
	Concepto        string
	Clasificaciones []string
}{
	{"Ingresos", []string{"Datafonos", "Depósitos de Clientes", "Asociaciones"}},
	{"Gastos", []string{"Electricidad", "Agua", "CCSS", "Planilla"}},
	{"Traslados de Fondos", []string{"Traslado entre cuentas"}},
	{"Overnight", []string{"Overnight"}},
}

// Run ejecuta la siembra completa de forma idempotente.
func Run(ctx context.Context, pool *pgxpool.Pool, log *zap.Logger, cfg config.Config) error {
	rolIDs := make(map[string]string, len(roles))
	for _, r := range roles {
		id, err := upsertID(ctx, pool,
			`INSERT INTO rol (codigo, nombre, descripcion) VALUES ($1, $2, $3)
			 ON CONFLICT (codigo) DO UPDATE SET nombre = EXCLUDED.nombre RETURNING id::text`,
			r.Codigo, r.Nombre, r.Desc)
		if err != nil {
			return fmt.Errorf("seed rol %s: %w", r.Codigo, err)
		}
		rolIDs[r.Codigo] = id
	}

	empresaIDs := make(map[string]string, len(empresas))
	for _, nombre := range empresas {
		id, err := upsertID(ctx, pool,
			`INSERT INTO empresa (nombre) VALUES ($1)
			 ON CONFLICT (nombre) DO UPDATE SET activo = true RETURNING id::text`, nombre)
		if err != nil {
			return fmt.Errorf("seed empresa %s: %w", nombre, err)
		}
		empresaIDs[nombre] = id
		if err := seedCatalogo(ctx, pool, id); err != nil {
			return fmt.Errorf("seed catálogo %s: %w", nombre, err)
		}
	}

	hash, err := auth.HashPassword(cfg.SeedPassword)
	if err != nil {
		return fmt.Errorf("seed hash admin: %w", err)
	}
	adminID, err := upsertID(ctx, pool,
		`INSERT INTO usuario (nombre, email, password_hash) VALUES ($1, $2, $3)
		 ON CONFLICT (email) DO UPDATE SET nombre = EXCLUDED.nombre RETURNING id::text`,
		"Administrador GPVDP", cfg.SeedEmail, hash)
	if err != nil {
		return fmt.Errorf("seed admin: %w", err)
	}

	for _, nombre := range empresas {
		if _, err := pool.Exec(ctx,
			`INSERT INTO usuario_empresa_rol (empresa_id, usuario_id, rol_id)
			 VALUES ($1::uuid, $2::uuid, $3::uuid)
			 ON CONFLICT (empresa_id, usuario_id) DO UPDATE SET rol_id = EXCLUDED.rol_id`,
			empresaIDs[nombre], adminID, rolIDs["ADMIN"]); err != nil {
			return fmt.Errorf("seed membership %s: %w", nombre, err)
		}
	}

	// Bancos y cuentas reales de Valle de Paz (docs/GPVDP_Formatos_Bancos_v1.0.md).
	if vid, ok := empresaIDs["Valle de Paz"]; ok {
		if err := seedCuentas(ctx, pool, vid); err != nil {
			return fmt.Errorf("seed cuentas Valle de Paz: %w", err)
		}
	}

	log.Info("seed completado",
		zap.Int("roles", len(roles)),
		zap.Int("empresas", len(empresas)),
		zap.Int("cuentas_vdp", len(cuentasVDP)),
		zap.String("admin", cfg.SeedEmail))
	return nil
}

var bancosVDP = []string{"Promerica", "BN", "BAC", "BCR", "Banco Popular", "Davivienda"}

// cuentasVDP: 13 cuentas reales de Valle de Paz. Alias único por empresa (clave de negocio).
var cuentasVDP = []struct{ Banco, Alias, IBAN, Moneda string }{
	{"Promerica", "Promerica Colinas", "", "CRC"},
	{"BN", "BN Jardines Colones", "", "CRC"},
	{"BN", "BN Jardines Dólares", "", "USD"},
	{"BN", "BN Privado de Cartago", "", "CRC"},
	{"BN", "BN Valle de Paz Colones", "", "CRC"},
	{"BN", "BN Valle de Paz Dólares", "", "USD"},
	{"BAC", "BAC Religiosa", "CR26010200009038253541", "CRC"},
	{"BAC", "BAC Valle de Paz Colones", "CR10010200009510302389", "CRC"},
	{"BCR", "BCR Religiosa", "CR48015201349000020206", "CRC"},
	{"Banco Popular", "BP Valle de Paz Colones", "CR62016101008810244232", "CRC"},
	{"Davivienda", "Davivienda Colones", "CR76010409142215626710", "CRC"},
	{"Davivienda", "Davivienda Comisiones COPENAE", "CR17010402842201520116", "CRC"},
	{"Davivienda", "Davivienda Dólares", "CR74010409142215627425", "USD"},
}

func seedCuentas(ctx context.Context, pool *pgxpool.Pool, empresaID string) error {
	bancoIDs := make(map[string]string, len(bancosVDP))
	for _, nombre := range bancosVDP {
		id, err := upsertID(ctx, pool,
			`INSERT INTO banco (empresa_id, nombre) VALUES ($1::uuid, $2)
			 ON CONFLICT (empresa_id, nombre) DO UPDATE SET activo = true RETURNING id::text`,
			empresaID, nombre)
		if err != nil {
			return fmt.Errorf("banco %s: %w", nombre, err)
		}
		bancoIDs[nombre] = id
	}
	for _, cta := range cuentasVDP {
		var ibanArg any // "" => NULL (evita colisión de unique(empresa_id, iban) entre cuentas sin IBAN)
		if cta.IBAN != "" {
			ibanArg = cta.IBAN
		}
		if _, err := pool.Exec(ctx,
			`INSERT INTO cuenta_bancaria (empresa_id, banco_id, iban, moneda, alias)
			 VALUES ($1::uuid, $2::uuid, $3, $4, $5)
			 ON CONFLICT (empresa_id, alias) DO UPDATE SET banco_id = EXCLUDED.banco_id, moneda = EXCLUDED.moneda`,
			empresaID, bancoIDs[cta.Banco], ibanArg, cta.Moneda, cta.Alias); err != nil {
			return fmt.Errorf("cuenta %s: %w", cta.Alias, err)
		}
	}
	return nil
}

// seedCatalogo siembra el catálogo de DEMOSTRACIÓN, y solo cuando la empresa todavía no tiene
// ninguno. Correr en cada arranque era destructivo, aunque el INSERT dijera ON CONFLICT DO NOTHING:
//
//   - El ON CONFLICT calza por NOMBRE EXACTO. Si el usuario fusionó «Depósitos de Clientes» hacia su
//     propia «Deposito de Clientes» (sin tilde), el seed no reconoce su nombre y la vuelve a
//     insertar. Verificado en la base: el usuario fusionó a las 17:27 y la clasificación reapareció
//     a las 18:01, con id nuevo y sin evento de auditoría, en el siguiente reinicio del backend.
//   - El concepto además hacía `DO UPDATE SET activo = true`, así que reactivaba lo que el usuario
//     hubiera desactivado.
//
// El catálogo es del usuario: fusionar, renombrar, desactivar y eliminar son decisiones suyas, y un
// seed de demo no puede deshacerlas cada vez que el servicio se reinicia. Con la guarda, el seed
// sigue sirviendo para lo que se hizo (una empresa nueva arranca con algo que mirar) y deja de
// pisar el trabajo real.
func seedCatalogo(ctx context.Context, pool *pgxpool.Pool, empresaID string) error {
	var tiene bool
	if err := pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM concepto WHERE empresa_id = $1::uuid)`, empresaID).Scan(&tiene); err != nil {
		return fmt.Errorf("seed catálogo: revisar si ya existe: %w", err)
	}
	if tiene {
		return nil // la empresa ya tiene catálogo: no se toca
	}
	for _, cd := range conceptosDemo {
		// `DO UPDATE SET nombre = EXCLUDED.nombre` en vez de DO NOTHING: es un no-op que igual
		// devuelve la fila, así que el RETURNING nunca queda vacío. Con DO NOTHING, un choque
		// devolvería cero filas y el Scan fallaría. Lo que NO hace es tocar `activo`: reactivar un
		// concepto que el usuario desactivó era parte del problema.
		cid, err := upsertID(ctx, pool,
			`INSERT INTO concepto (empresa_id, nombre) VALUES ($1::uuid, $2)
			 ON CONFLICT (empresa_id, nombre) DO UPDATE SET nombre = EXCLUDED.nombre RETURNING id::text`,
			empresaID, cd.Concepto)
		if err != nil {
			return err
		}
		for _, cl := range cd.Clasificaciones {
			if _, err := pool.Exec(ctx,
				`INSERT INTO clasificacion (empresa_id, concepto_id, nombre) VALUES ($1::uuid, $2::uuid, $3)
				 ON CONFLICT (empresa_id, concepto_id, nombre) DO NOTHING`,
				empresaID, cid, cl); err != nil {
				return err
			}
		}
	}
	return nil
}

func upsertID(ctx context.Context, pool *pgxpool.Pool, q string, args ...any) (string, error) {
	var id string
	if err := pool.QueryRow(ctx, q, args...).Scan(&id); err != nil {
		return "", err
	}
	return id, nil
}
