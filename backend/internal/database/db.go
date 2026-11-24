// Package database gestiona el pool de conexiones y las migraciones PostgreSQL.
package database

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// maxConnsPorDefecto es el techo de conexiones simultáneas a la base.
//
// SUBIÓ DE 10 A 25 (9-nov-2026). Diez era el techo de TODO el backend, y cada pantalla del ERP
// dispara entre 9 y 12 peticiones al abrirse: con tres personas entrando a la vez ya había cola,
// y esa espera no se ve en ninguna métrica de la base —la consulta es rápida, lo que tarda es
// conseguir turno para ejecutarla—.
//
// 25 contra el max_connections de PostgreSQL (100 por defecto) deja sitio de sobra para las
// migraciones del arranque, psql y cualquier herramienta de mantenimiento. Subirlo más no acelera
// nada por sí solo: pasado el número de núcleos de la base, las consultas se pelean la CPU en vez
// de esperar en la fila.
const maxConnsPorDefecto = 25

// minConnsPorDefecto deja conexiones ya abiertas y listas.
//
// Abrir una conexión cuesta ida y vuelta de red más autenticación. Con el mínimo en cero, la
// primera persona de la mañana —y cada pico tras un rato de calma— pagaba esa apertura dentro de
// su propia pantalla.
const minConnsPorDefecto = 5

// NewPool crea y verifica un pool de conexiones pgx contra PostgreSQL.
//
// El techo se puede mover con DB_MAX_CONNS sin recompilar: el servidor de producción puede tener
// otra relación entre núcleos y usuarios que la máquina de desarrollo. Un valor inválido o fuera
// de rango se ignora en silencio y queda el de por defecto, porque un pool mal configurado por un
// dedazo en el `.env` no debe impedir que el sistema levante.
func NewPool(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("database: parse config: %w", err)
	}
	cfg.MaxConns = maxConns()
	cfg.MinConns = minConnsPorDefecto
	if cfg.MaxConns < cfg.MinConns {
		cfg.MinConns = cfg.MaxConns
	}
	cfg.MaxConnIdleTime = 5 * time.Minute
	// Reciclar la conexión cada hora: una conexión eterna acumula memoria del lado del servidor
	// (planes cacheados, tablas temporales) y nunca se entera de un cambio de configuración.
	cfg.MaxConnLifetime = time.Hour

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("database: nuevo pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("database: ping: %w", err)
	}
	return pool, nil
}

// maxConns resuelve el techo de conexiones: DB_MAX_CONNS si trae un entero entre 1 y 200, o el
// valor por defecto. El tope de 200 es un pasamanos contra el dedazo —un `DB_MAX_CONNS=2500`
// haría que el pool intente abrir más conexiones de las que PostgreSQL acepta y el backend
// fallaría a mitad de jornada, no al arrancar, que es la peor forma de enterarse—.
func maxConns() int32 {
	v, err := strconv.Atoi(os.Getenv("DB_MAX_CONNS"))
	if err != nil || v < 1 || v > 200 {
		return maxConnsPorDefecto
	}
	return int32(v)
}
