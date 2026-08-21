#!/usr/bin/env bash
#
# Respaldo de la base. Lo corre el cron todas las noches y también se puede correr a mano:
#
#   bash deploy/respaldar.sh
#
# Se puede ejecutar con gente trabajando: pg_dump toma una foto consistente y no bloquea.
#
set -euo pipefail

RAIZ="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$RAIZ"
COMPOSE="docker compose -f deploy/docker-compose.prod.yml"
DESTINO="$RAIZ/respaldos"
DIAS_A_GUARDAR="${DIAS_A_GUARDAR:-14}"

[[ -f .env ]] || { echo "ERROR: falta el archivo .env"; exit 1; }
set -a; . ./.env; set +a

mkdir -p "$DESTINO"; chmod 700 "$DESTINO"
SELLO="$(date +%Y%m%d_%H%M)"
ARCHIVO="$DESTINO/gpvdp_${SELLO}.dump"

echo "[$(date -Is)] respaldando…"

# -Fc = formato comprimido de PostgreSQL: más chico y permite restaurar tablas sueltas.
# Se escribe primero dentro del contenedor y luego se copia, para no depender de cómo el shell
# maneje los datos binarios en una redirección.
$COMPOSE exec -T db pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc -f /tmp/respaldo.dump
$COMPOSE cp db:/tmp/respaldo.dump "$ARCHIVO"
$COMPOSE exec -T db rm -f /tmp/respaldo.dump
chmod 600 "$ARCHIVO"

TAMANO=$(du -h "$ARCHIVO" | cut -f1)

# Verificar que el archivo sirve, no solo que existe. Un respaldo ilegible es peor que ninguno,
# porque da una falsa sensación de seguridad.
if $COMPOSE cp "$ARCHIVO" db:/tmp/verificar.dump >/dev/null 2>&1 \
	&& $COMPOSE exec -T db pg_restore -l /tmp/verificar.dump >/dev/null 2>&1; then
	$COMPOSE exec -T db rm -f /tmp/verificar.dump
	echo "[$(date -Is)] OK: $ARCHIVO ($TAMANO) — verificado legible"
else
	echo "[$(date -Is)] ATENCIÓN: $ARCHIVO se creó pero NO se pudo verificar. Revisalo a mano."
	exit 1
fi

# Rotación: se borran los más viejos que DIAS_A_GUARDAR.
BORRADOS=$(find "$DESTINO" -name 'gpvdp_*.dump' -type f -mtime "+${DIAS_A_GUARDAR}" -print -delete | wc -l)
(( BORRADOS > 0 )) && echo "[$(date -Is)] rotación: $BORRADOS respaldo(s) de más de ${DIAS_A_GUARDAR} días eliminados"

CUANTOS=$(find "$DESTINO" -name 'gpvdp_*.dump' -type f | wc -l)
echo "[$(date -Is)] hay $CUANTOS respaldo(s) guardados en $DESTINO"

# Recordatorio que importa: un respaldo que vive solo en el mismo servidor no protege contra
# perder el servidor. Conviene copiarlo afuera (otro proveedor, S3/Spaces, o descargarlo).
if [[ -z "${RESPALDO_EXTERNO_LISTO:-}" ]]; then
	echo "[$(date -Is)] NOTA: estos respaldos están en el MISMO servidor. Copialos afuera."
fi
