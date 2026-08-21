#!/usr/bin/env bash
#
# Poner al día el paquete `entrega-programadores/` — el que reciben los programadores externos.
#
# POR QUÉ EXISTE
#
# Tres de los cinco archivos de ese paquete no se escriben a mano: salen del esquema de la base y
# de las rutas del código. Se generaron una vez y quedaron desfasados en silencio, que es la peor
# forma de quedar mal: `schema.sql` sin una columna nueva se corre sin dar error y la aplicación
# falla después, en una pantalla cualquiera, sin que nadie relacione las dos cosas.
#
# QUÉ HACE, Y QUÉ NO
#
#   · `schema.sql` se REGENERA completo. Es puro pg_dump: no hay nada editorial que perder.
#   · `ENDPOINTS.md` y `DICCIONARIO-DATOS.md` se VERIFICAN, no se reescriben. Los dos tienen
#     agrupaciones y explicaciones escritas por una persona («Salud del servicio», las convenciones
#     de tipos) que un generador destruiría. El script dice EXACTAMENTE qué falta agregar y
#     termina con error si falta algo, así se puede usar como control antes de entregar.
#
# USO (desde la raíz del proyecto, con el sistema levantado):
#
#   bash deploy/regenerar-entrega.sh                      # usa el compose de desarrollo
#   bash deploy/regenerar-entrega.sh deploy/docker-compose.prod.yml   # usa el de producción
#
set -euo pipefail

verde() { printf '\033[32m%s\033[0m\n' "$*"; }
rojo()  { printf '\033[31m%s\033[0m\n' "$*"; }
ama()   { printf '\033[33m%s\033[0m\n' "$*"; }
titulo() { printf '\n\033[1m── %s ──\033[0m\n' "$*"; }

RAIZ="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$RAIZ"

COMPOSE_FILE="${1:-docker-compose.yml}"
COMPOSE="docker compose -f $COMPOSE_FILE"
ENTREGA="$RAIZ/entrega-programadores"
ROUTER="$RAIZ/backend/internal/server/router.go"

[[ -d "$ENTREGA" ]] || { rojo "no existe $ENTREGA"; exit 1; }
[[ -f "$ROUTER" ]]  || { rojo "no existe $ROUTER"; exit 1; }

# Las credenciales salen del .env si existe (producción); si no, de los valores de desarrollo.
if [[ -f "$RAIZ/.env" ]]; then set -a; . "$RAIZ/.env"; set +a; fi
DB_USER="${POSTGRES_USER:-gpvdp}"
DB_NAME="${POSTGRES_DB:-gpvdp}"

PSQL=(env MSYS_NO_PATHCONV=1 $COMPOSE exec -T db psql -U "$DB_USER" -d "$DB_NAME" -tAc)

titulo "1. schema.sql (se regenera)"
# --schema-only: solo la estructura, sin datos de la empresa. --no-owner y --no-privileges: para
# que se pueda correr con cualquier usuario en la base del que lo reciba.
env MSYS_NO_PATHCONV=1 $COMPOSE exec -T db \
	pg_dump -U "$DB_USER" -d "$DB_NAME" --schema-only --no-owner --no-privileges \
	> "$ENTREGA/schema.sql.nuevo"

if [[ ! -s "$ENTREGA/schema.sql.nuevo" ]]; then
	rm -f "$ENTREGA/schema.sql.nuevo"
	rojo "  el volcado salió vacío: ¿está la base levantada?"
	exit 1
fi
TABLAS=$(grep -c '^CREATE TABLE' "$ENTREGA/schema.sql.nuevo" || true)
mv "$ENTREGA/schema.sql.nuevo" "$ENTREGA/schema.sql"
verde "  schema.sql regenerado: $TABLAS tablas, $(wc -l < "$ENTREGA/schema.sql") líneas."

titulo "2. ENDPOINTS.md (se verifica)"
# Las rutas del código, tal como las registra Gin. Se toma el método y la ruta literal.
RUTAS_CODIGO="$(mktemp)"
grep -oE '\.(GET|POST|PUT|PATCH|DELETE)\("[^"]+"' "$ROUTER" \
	| sed -E 's/^\.([A-Z]+)\("/\1 /; s/"$//' \
	| sort -u > "$RUTAS_CODIGO"
TOTAL_CODIGO=$(wc -l < "$RUTAS_CODIGO")
verde "  el código registra $TOTAL_CODIGO ruta(s) distintas."

FALTAN_RUTAS=0
while read -r metodo ruta; do
	# En el documento las rutas van con el prefijo /v1 y entre acentos graves.
	if ! grep -qF "\`/v1${ruta}\`" "$ENTREGA/ENDPOINTS.md" && ! grep -qF "\`${ruta}\`" "$ENTREGA/ENDPOINTS.md"; then
		ama "  FALTA en el documento:  $metodo /v1${ruta}"
		FALTAN_RUTAS=$((FALTAN_RUTAS + 1))
	fi
done < "$RUTAS_CODIGO"
rm -f "$RUTAS_CODIGO"
if (( FALTAN_RUTAS == 0 )); then
	verde "  ENDPOINTS.md está completo."
else
	ama "  $FALTAN_RUTAS ruta(s) sin documentar (arriba). Agregalas a su sección del documento."
fi

titulo "3. DICCIONARIO-DATOS.md (se verifica)"
COLS_DB="$(mktemp)"
"${PSQL[@]}" "
	SELECT c.table_name || '.' || c.column_name
	FROM information_schema.columns c
	JOIN information_schema.tables t
	  ON t.table_schema = c.table_schema AND t.table_name = c.table_name
	 AND t.table_type = 'BASE TABLE'
	WHERE c.table_schema = 'public' AND c.table_name <> 'schema_migrations'
	ORDER BY 1" > "$COLS_DB"
TOTAL_COLS=$(grep -c . "$COLS_DB" || true)
TOTAL_TABLAS=$("${PSQL[@]}" "
	SELECT count(*) FROM information_schema.tables
	WHERE table_schema='public' AND table_type='BASE TABLE' AND table_name <> 'schema_migrations'")
verde "  la base tiene $TOTAL_TABLAS tabla(s) y $TOTAL_COLS columna(s)."

FALTAN_COLS=0
while IFS='.' read -r tabla columna; do
	[[ -z "${columna:-}" ]] && continue
	if ! grep -qF "\`${columna}\`" "$ENTREGA/DICCIONARIO-DATOS.md"; then
		ama "  FALTA en el diccionario:  ${tabla}.${columna}"
		FALTAN_COLS=$((FALTAN_COLS + 1))
	fi
done < "$COLS_DB"
rm -f "$COLS_DB"

# El encabezado del documento declara los totales: se actualiza siempre, porque un conteo viejo
# es una afirmación falsa aunque el resto del documento esté bien.
if grep -qE '^\*\*[0-9]+ tablas · [0-9]+ columnas\*\*$' "$ENTREGA/DICCIONARIO-DATOS.md"; then
	sed -i -E "s/^\*\*[0-9]+ tablas · [0-9]+ columnas\*\*$/**${TOTAL_TABLAS} tablas · ${TOTAL_COLS} columnas**/" \
		"$ENTREGA/DICCIONARIO-DATOS.md"
	verde "  encabezado actualizado a ${TOTAL_TABLAS} tablas · ${TOTAL_COLS} columnas."
fi
if (( FALTAN_COLS == 0 )); then
	verde "  DICCIONARIO-DATOS.md está completo."
else
	ama "  $FALTAN_COLS columna(s) sin documentar (arriba)."
fi

# Nota: la verificación de columnas es por NOMBRE, no por tabla.columna. Un nombre repetido en otra
# tabla (empresa_id, activo, creado_en) puede dar por documentada una columna que no lo está. Es a
# propósito: el diccionario agrupa por tabla y el objetivo acá es detectar nombres NUEVOS, que es
# lo que se olvida. Para una revisión exhaustiva, mirar la sección de la tabla que cambió.

printf '\n'
if (( FALTAN_RUTAS == 0 && FALTAN_COLS == 0 )); then
	verde "El paquete de entrega está al día."
	exit 0
fi
rojo "El paquete NO está al día: faltan $FALTAN_RUTAS ruta(s) y $FALTAN_COLS columna(s)."
rojo "schema.sql SÍ quedó regenerado; los otros dos hay que completarlos a mano donde corresponde."
exit 1
