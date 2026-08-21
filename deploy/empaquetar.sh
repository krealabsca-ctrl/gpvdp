#!/usr/bin/env bash
#
# Armar UN archivo con todo lo necesario para instalar el ERP en un servidor.
#
# POR QUÉ EXISTE
#
# El paquete que se entregaba antes (`entrega-programadores/`) son cinco documentos: manual,
# esquema, diccionario y catálogo de rutas. **Cero líneas de código.** Sirve para entender el
# sistema, no para levantarlo. Y `deploy/` sola tampoco alcanza: el instalador COMPILA el backend
# y el frontend desde el código fuente (el compose construye desde `../backend` y `../frontend`).
#
# Este script produce el archivo que sí se puede instalar: código fuente completo + scripts +
# documentación, en un solo `.tar.gz` que se copia al servidor y se descomprime.
#
# QUÉ DEJA AFUERA, Y POR QUÉ IMPORTA
#
#   · `respaldos/`  — son volcados de la base con los movimientos bancarios REALES de las empresas.
#                     Mandarlos en un paquete de despliegue es filtrar los datos de la operación.
#   · `.env`        — las claves de la instalación. El instalador genera las suyas en el servidor.
#   · `node_modules`, `dist`, `backend/bin` — se regeneran al compilar; solo agregan peso.
#   · `.claude/`    — configuración de la herramienta de desarrollo, no hace falta para instalar.
#
# La carpeta que queda al descomprimir se llama `gpvdp-erp`, con UN solo nivel: la instalación
# actual tiene dos carpetas anidadas con el mismo nombre y eso ya causó confusión sobre cuál copiar.
#
# USO (desde la raíz del proyecto):
#
#   bash deploy/empaquetar.sh              # deja el paquete en ../ (junto al proyecto)
#   bash deploy/empaquetar.sh /otra/ruta   # o donde se le diga
#
set -euo pipefail

verde() { printf '\033[32m%s\033[0m\n' "$*"; }
rojo()  { printf '\033[31m%s\033[0m\n' "$*"; }
ama()   { printf '\033[33m%s\033[0m\n' "$*"; }
titulo() { printf '\n\033[1m── %s ──\033[0m\n' "$*"; }
morir() { rojo "ERROR: $*"; exit 1; }

RAIZ="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$RAIZ"

DESTINO="${1:-$(cd "$RAIZ/.." && pwd)}"
mkdir -p "$DESTINO"
NOMBRE_RAIZ="gpvdp-erp"
SELLO="$(date +%Y%m%d)"
PAQUETE="$DESTINO/gpvdp-erp-$SELLO.tar.gz"

# ─────────────────────── lo que entra y lo que no ───────────────────────
# Se listan las carpetas a INCLUIR, no las a excluir: si mañana aparece una carpeta nueva con
# datos o secretos, no se cuela sola en el paquete.
INCLUIR=(
	backend            # código Go, migraciones embebidas, Dockerfile
	frontend           # código React/TS, Dockerfile.prod
	deploy             # instalador, actualizador, respaldo, Caddyfile, compose de producción
	docs               # formatos de banco y especificaciones
	entrega-programadores # manual técnico, esquema, diccionario, endpoints
	infra              # configuración auxiliar
	docker-compose.yml # ambiente de desarrollo
	Makefile
	CLAUDE.md          # las reglas del proyecto que no se adivinan leyendo el código
	README.md
)

EXCLUIR=(
	--exclude=node_modules
	--exclude=dist
	--exclude=bin
	--exclude=.git
	--exclude=.env
	--exclude=.env.local
	--exclude=.env.prueba
	--exclude='*.dump'
	--exclude='*.log'
	--exclude=.DS_Store
	--exclude='desktop.ini'
)

titulo "1. Revisando qué hay para empaquetar"
for x in "${INCLUIR[@]}"; do
	[[ -e "$x" ]] || morir "falta '$x' en la raíz del proyecto: ¿se está corriendo desde el lugar correcto?"
done
GO=$(find backend -name '*.go' -type f | wc -l)
MIG=$(find backend/migrations -name '*.up.sql' -type f | wc -l)
TS=$(find frontend/src -type f \( -name '*.ts' -o -name '*.tsx' \) | wc -l)
verde "  backend: $GO archivos .go · $MIG migraciones"
verde "  frontend: $TS archivos .ts/.tsx"
(( GO > 100 && MIG > 50 && TS > 50 )) || morir "el conteo de archivos es sospechosamente bajo; no se empaqueta a ciegas"

titulo "2. Armando el paquete"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
mkdir -p "$TMP/$NOMBRE_RAIZ"
tar -cf - "${EXCLUIR[@]}" "${INCLUIR[@]}" | tar -xf - -C "$TMP/$NOMBRE_RAIZ"

# El instalador y sus compañeros tienen que quedar ejecutables aunque el archivo se haya armado
# desde Windows, donde el permiso de ejecución no existe.
chmod +x "$TMP/$NOMBRE_RAIZ"/deploy/*.sh

tar -czf "$PAQUETE" -C "$TMP" "$NOMBRE_RAIZ"
TAMANO=$(du -h "$PAQUETE" | cut -f1)
verde "  $PAQUETE ($TAMANO)"

titulo "3. Comprobando el paquete (no basta con que exista)"
VER="$(mktemp -d)"
tar -xzf "$PAQUETE" -C "$VER"
FALLAS=0

# Los archivos sin los que la instalación no arranca.
for f in \
	"$NOMBRE_RAIZ/deploy/instalar-vps.sh" \
	"$NOMBRE_RAIZ/deploy/docker-compose.prod.yml" \
	"$NOMBRE_RAIZ/deploy/Caddyfile" \
	"$NOMBRE_RAIZ/backend/go.mod" \
	"$NOMBRE_RAIZ/backend/Dockerfile" \
	"$NOMBRE_RAIZ/backend/cmd/api/main.go" \
	"$NOMBRE_RAIZ/frontend/package.json" \
	"$NOMBRE_RAIZ/frontend/Dockerfile.prod" \
	"$NOMBRE_RAIZ/frontend/index.html"
do
	if [[ ! -f "$VER/$f" ]]; then rojo "  FALTA: $f"; FALLAS=$((FALLAS+1)); fi
done

# Los conteos tienen que sobrevivir el viaje.
GO2=$(find "$VER/$NOMBRE_RAIZ/backend" -name '*.go' -type f | wc -l)
MIG2=$(find "$VER/$NOMBRE_RAIZ/backend/migrations" -name '*.up.sql' -type f | wc -l)
TS2=$(find "$VER/$NOMBRE_RAIZ/frontend/src" -type f \( -name '*.ts' -o -name '*.tsx' \) | wc -l)
[[ "$GO2" == "$GO"  ]] || { rojo "  .go: $GO2 en el paquete vs $GO en el proyecto"; FALLAS=$((FALLAS+1)); }
[[ "$MIG2" == "$MIG" ]] || { rojo "  migraciones: $MIG2 vs $MIG"; FALLAS=$((FALLAS+1)); }
[[ "$TS2" == "$TS"  ]] || { rojo "  .ts/.tsx: $TS2 vs $TS"; FALLAS=$((FALLAS+1)); }
(( FALLAS == 0 )) && verde "  el código llegó completo: $GO2 .go · $MIG2 migraciones · $TS2 .ts/.tsx"

# Y lo que NO tiene que estar. Esta parte importa más que la anterior: un paquete al que se le
# colaron los respaldos o el .env filtra los datos y las claves de la operación.
COLADOS=0
while read -r sospechoso; do
	[[ -z "$sospechoso" ]] && continue
	rojo "  SE COLÓ: $sospechoso"
	COLADOS=$((COLADOS+1))
done < <(cd "$VER" && find . \( -name '*.dump' -o -name '.env' -o -name 'node_modules' -o -name '*.sql' -path '*respaldos*' \) -print)
if (( COLADOS == 0 )); then
	verde "  no viajan respaldos, ni .env, ni dependencias compiladas."
else
	morir "$COLADOS archivo(s) que no deben salir del servidor están en el paquete. NO se entrega así."
fi
rm -rf "$VER"

(( FALLAS == 0 )) || morir "el paquete está incompleto (ver arriba)"

# ─────────────────────────────── resumen ───────────────────────────────
printf '\n\033[1m═══════════════════════════════════════════════════════\033[0m\n'
verde " Paquete listo"
printf '\033[1m═══════════════════════════════════════════════════════\033[0m\n\n'
cat <<INSTRUCCIONES
  Archivo:  $PAQUETE
  Tamaño:   $TAMANO

  Instalar en el servidor, tres comandos:

    1) Subirlo (desde tu computadora):
       scp "$PAQUETE" root@LA-IP-DEL-SERVIDOR:/opt/

    2) Entrar y descomprimir:
       ssh root@LA-IP-DEL-SERVIDOR
       cd /opt && tar -xzf gpvdp-erp-$SELLO.tar.gz && cd $NOMBRE_RAIZ

    3) Instalar (con el dominio ya apuntando al servidor):
       bash deploy/instalar-vps.sh erp.tudominio.com tucorreo@tudominio.com

  Sin dominio todavía (HTTP por IP, solo para probar):
       bash deploy/instalar-vps.sh --sin-dominio

  El instalador compila todo, crea las claves al azar, siembra la base la primera vez, cierra el
  firewall, programa el respaldo diario e imprime la contraseña temporal del administrador.

  Detalle completo: $NOMBRE_RAIZ/deploy/LEEME-VPS.md

INSTRUCCIONES
