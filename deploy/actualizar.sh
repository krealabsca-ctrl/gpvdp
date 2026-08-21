#!/usr/bin/env bash
#
# Publicar cambios en el servidor. Es el comando que van a usar los programadores:
#
#   bash deploy/actualizar.sh
#
# Qué hace, en este orden y por esta razón:
#   1. Respalda ANTES de tocar nada. Si una migración sale mal, hay a dónde volver.
#   2. Compila las imágenes nuevas mientras el sistema sigue funcionando.
#   3. Recién cuando la compilación terminó bien, cambia los contenedores.
#   4. Comprueba que responde y avisa si no.
#
# El corte de servicio son los segundos que tarda el paso 3, no toda la compilación.
#
set -euo pipefail

verde() { printf '\033[32m%s\033[0m\n' "$*"; }
rojo()  { printf '\033[31m%s\033[0m\n' "$*"; }
ama()   { printf '\033[33m%s\033[0m\n' "$*"; }
titulo() { printf '\n\033[1m── %s ──\033[0m\n' "$*"; }

RAIZ="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$RAIZ"
COMPOSE="docker compose -f deploy/docker-compose.prod.yml"

[[ -f .env ]] || { rojo "ERROR: falta .env — ¿corriste deploy/instalar-vps.sh?"; exit 1; }
set -a; . ./.env; set +a

titulo "1. Respaldo previo"
bash deploy/respaldar.sh

titulo "2. Versión de la base ANTES"
ANTES="$($COMPOSE exec -T db psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -tAc \
	"SELECT version || CASE WHEN dirty THEN ' (SUCIA)' ELSE '' END FROM schema_migrations" 2>/dev/null || echo '?')"
echo "  migración: $ANTES"
if [[ "$ANTES" == *SUCIA* ]]; then
	rojo "  La base quedó en estado inconsistente por una migración que falló a medias."
	rojo "  NO se puede actualizar así. Hay que arreglar el SQL y limpiar la marca antes."
	exit 1
fi

titulo "3. Compilando (el sistema sigue en línea)"
$COMPOSE build

titulo "4. Cambiando a la versión nueva"
# El backend aplica las migraciones pendientes al arrancar.
$COMPOSE up -d
verde "  Contenedores actualizados."

titulo "5. Comprobando"
OK=0
for i in $(seq 1 45); do
	if $COMPOSE exec -T backend wget -qO- http://127.0.0.1:8080/v1/healthz 2>/dev/null | grep -q ok; then OK=1; break; fi
	sleep 2
done
if (( ! OK )); then
	rojo "  El backend NO responde después de actualizar."
	rojo "  Mirá qué pasó:   $COMPOSE logs --tail 50 backend"
	rojo "  Para volver atrás: revisá deploy/LEEME-VPS.md, sección «Si una actualización sale mal»."
	exit 1
fi
verde "  El backend responde."

DESPUES="$($COMPOSE exec -T db psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -tAc \
	"SELECT version || CASE WHEN dirty THEN ' (SUCIA)' ELSE '' END FROM schema_migrations" 2>/dev/null || echo '?')"
echo "  migración: $ANTES → $DESPUES"
if [[ "$DESPUES" == *SUCIA* ]]; then
	rojo "  ATENCIÓN: una migración falló a medias y la base quedó marcada como sucia."
	rojo "  El sistema puede estar respondiendo con el esquema incompleto. Revisalo YA."
	exit 1
fi

# El navegador de cada usuario puede tener guardada la versión anterior de la aplicación.
# El Caddyfile marca index.html como no-cacheable justamente para esto, pero conviene avisar.
printf '\n'
verde "Actualización terminada."
ama "Si alguien reporta algo raro, que recargue con Ctrl+Shift+R: puede tener guardada la versión anterior."
