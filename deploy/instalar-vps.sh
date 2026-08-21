#!/usr/bin/env bash
#
# GPVDP ERP — instalador para un VPS (DigitalOcean, GoDaddy VPS, Hetzner, Linode…).
#
# Qué hace, en orden:
#   1. Comprueba que el servidor sirve (sistema, permisos, memoria).
#   2. Instala Docker si no está.
#   3. Pide el dominio y el correo, o los toma de los argumentos.
#   4. Genera los secretos (contraseñas y llaves) al azar, distintos en cada instalación.
#   5. Cierra el servidor con firewall, dejando solo SSH y web.
#   6. Compila y levanta el sistema.
#   7. Siembra los datos iniciales y APAGA la siembra, para que no vuelva a correr.
#   8. Programa el respaldo diario.
#   9. Imprime la contraseña del administrador UNA vez.
#
# Se puede volver a correr sin miedo: detecta lo que ya está hecho y no lo repite. No borra
# datos nunca.
#
# USO (dentro del servidor, como root, desde la raíz del proyecto):
#
#   bash deploy/instalar-vps.sh erp.midominio.com admin@midominio.com
#
# Sin dominio todavía (queda en HTTP por la IP, para probar):
#
#   bash deploy/instalar-vps.sh --sin-dominio
#
set -euo pipefail

# ─────────────────────────────── presentación ───────────────────────────────
rojo()  { printf '\033[31m%s\033[0m\n' "$*"; }
verde() { printf '\033[32m%s\033[0m\n' "$*"; }
ama()   { printf '\033[33m%s\033[0m\n' "$*"; }
titulo() { printf '\n\033[1m── %s ──\033[0m\n' "$*"; }
morir() { rojo "ERROR: $*"; exit 1; }

RAIZ="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$RAIZ"
COMPOSE="docker compose -f deploy/docker-compose.prod.yml"
ENV_FILE="$RAIZ/.env"

# ─────────────────────────────── argumentos ───────────────────────────────
DOMINIO_ARG="" ; CORREO_ARG="" ; SIN_DOMINIO=0
while [[ $# -gt 0 ]]; do
	case "$1" in
		--sin-dominio) SIN_DOMINIO=1; shift ;;
		-h|--help) sed -n '2,30p' "$0"; exit 0 ;;
		*) if [[ -z "$DOMINIO_ARG" ]]; then DOMINIO_ARG="$1"; else CORREO_ARG="$1"; fi; shift ;;
	esac
done

titulo "1. Revisando el servidor"

[[ "$(id -u)" -eq 0 ]] || morir "hay que correrlo como root (probá: sudo bash deploy/instalar-vps.sh ...)"

# Sistema operativo. Se soporta Debian/Ubuntu porque es lo que traen por defecto DigitalOcean
# y los VPS de GoDaddy; en otra distribución habría que cambiar la instalación de Docker.
if [[ -r /etc/os-release ]]; then . /etc/os-release; else morir "no se pudo leer /etc/os-release"; fi
case "${ID:-}${ID_LIKE:-}" in
	*debian*|*ubuntu*) verde "  Sistema: ${PRETTY_NAME}" ;;
	*) morir "este instalador es para Debian o Ubuntu; el servidor dice: ${PRETTY_NAME:-desconocido}" ;;
esac

# Memoria. El paso que más memoria consume es compilar el frontend: con menos de ~2 GB el
# compilador se queda sin memoria y muere sin explicar por qué. Se cubre con swap.
MEM_MB=$(awk '/MemTotal/ {printf "%d", $2/1024}' /proc/meminfo)
verde "  Memoria: ${MEM_MB} MB"
if (( MEM_MB < 1900 )); then
	if swapon --show --noheadings | grep -q .; then
		verde "  Ya hay memoria de intercambio (swap) configurada."
	else
		ama "  Menos de 2 GB de memoria: creando 2 GB de swap para que la compilación no falle."
		fallocate -l 2G /swapfile || dd if=/dev/zero of=/swapfile bs=1M count=2048
		chmod 600 /swapfile && mkswap /swapfile >/dev/null && swapon /swapfile
		grep -q '^/swapfile' /etc/fstab || echo '/swapfile none swap sw 0 0' >> /etc/fstab
		verde "  Swap activado (y queda al reiniciar)."
	fi
fi

# Disco: la base, las imágenes y los respaldos necesitan espacio.
DISCO_GB=$(df -BG --output=avail / | tail -1 | tr -dc '0-9')
verde "  Disco libre: ${DISCO_GB} GB"
(( DISCO_GB >= 10 )) || morir "hacen falta al menos 10 GB libres; hay ${DISCO_GB} GB"

titulo "2. Instalando Docker"
# `curl` se usa más adelante para averiguar la IP pública y para comprobar el sitio a través de
# Caddy. Una imagen mínima de Debian no lo trae, y sin él el instalador fallaría a mitad de camino
# con un «command not found» en vez de una explicación.
if ! command -v curl >/dev/null 2>&1; then
	ama "  Instalando curl (hace falta para las comprobaciones)…"
	apt-get update -qq && apt-get install -y -qq curl >/dev/null
fi
if command -v docker >/dev/null 2>&1 && docker compose version >/dev/null 2>&1; then
	verde "  Docker ya está instalado: $(docker --version)"
else
	ama "  Instalando Docker desde el repositorio oficial…"
	apt-get update -qq
	apt-get install -y -qq ca-certificates curl gnupg >/dev/null
	install -m 0755 -d /etc/apt/keyrings
	curl -fsSL "https://download.docker.com/linux/${ID}/gpg" -o /etc/apt/keyrings/docker.asc
	chmod a+r /etc/apt/keyrings/docker.asc
	echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/${ID} ${VERSION_CODENAME} stable" \
		> /etc/apt/sources.list.d/docker.list
	apt-get update -qq
	apt-get install -y -qq docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin >/dev/null
	systemctl enable --now docker
	verde "  Docker instalado: $(docker --version)"
fi

titulo "3. Dominio"
if (( SIN_DOMINIO )); then
	DOMINIO=":80"
	IP_PUBLICA="$(curl -fsS --max-time 5 https://api.ipify.org || echo 'la IP del servidor')"
	ama "  Modo sin dominio: el sistema va a responder por HTTP en http://${IP_PUBLICA}"
	ama "  ATENCIÓN: sin dominio no hay HTTPS, así que las contraseñas viajan SIN CIFRAR por"
	ama "  internet. Sirve para una prueba corta; NO para uso real con datos de la empresa."
	CORS_ORIGINS="http://${IP_PUBLICA}"
else
	DOMINIO="${DOMINIO_ARG}"
	if [[ -z "$DOMINIO" ]]; then
		read -rp "  Dominio del sistema (ej: erp.valledepazcr.com): " DOMINIO
	fi
	[[ -n "$DOMINIO" ]] || morir "hace falta un dominio (o usá --sin-dominio)"
	[[ "$DOMINIO" =~ ^[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$ ]] || morir "el dominio '$DOMINIO' no tiene forma válida"

	CORREO="${CORREO_ARG}"
	if [[ -z "$CORREO" ]]; then
		read -rp "  Correo para los avisos del certificado: " CORREO
	fi
	[[ "$CORREO" == *@*.* ]] || morir "el correo '$CORREO' no tiene forma válida"

	# El certificado solo se puede emitir si el dominio ya apunta a este servidor. Comprobarlo
	# ahora evita que Caddy falle y consuma intentos: Let's Encrypt permite 5 por semana.
	IP_PUBLICA="$(curl -fsS --max-time 5 https://api.ipify.org || true)"
	IP_DOMINIO="$(getent hosts "$DOMINIO" | awk '{print $1; exit}' || true)"
	if [[ -n "$IP_PUBLICA" && -n "$IP_DOMINIO" && "$IP_PUBLICA" != "$IP_DOMINIO" ]]; then
		ama "  El dominio $DOMINIO apunta a $IP_DOMINIO, pero este servidor es $IP_PUBLICA."
		ama "  Hasta que el DNS apunte acá, el certificado no se va a poder emitir."
		read -rp "  ¿Continuar igual? (s/N): " seguir
		[[ "${seguir,,}" == "s" ]] || morir "corregí el DNS y volvé a correr el instalador"
	elif [[ -z "$IP_DOMINIO" ]]; then
		ama "  $DOMINIO todavía no resuelve. El certificado se emitirá cuando el DNS propague."
	else
		verde "  $DOMINIO apunta a este servidor ($IP_DOMINIO)."
	fi
	CORS_ORIGINS="https://${DOMINIO}"
fi

titulo "4. Secretos"
# Cada instalación genera los suyos. Nunca van escritos en el repositorio ni en este script.
if [[ -f "$ENV_FILE" ]]; then
	verde "  Ya existe .env: se conservan los secretos actuales (no se rota nada)."
	ama "  Si querés secretos nuevos, borrá $ENV_FILE y volvé a correr esto."
	# Solo se refresca lo que depende del dominio.
	sed -i "s|^DOMINIO=.*|DOMINIO=${DOMINIO}|" "$ENV_FILE"
	sed -i "s|^CORS_ORIGINS=.*|CORS_ORIGINS=${CORS_ORIGINS}|" "$ENV_FILE"
	# shellcheck disable=SC1090
	set -a; . "$ENV_FILE"; set +a
	ADMIN_PASS_NUEVA=""
else
	secreto() { openssl rand -base64 "${1:-48}" | tr -d '\n=+/' | cut -c1-"${2:-40}"; }
	POSTGRES_PASSWORD="$(secreto 48 32)"
	JWT_SECRET="$(secreto 64 64)"
	ADMIN_PASS_NUEVA="$(secreto 24 16)"
	SEED_ADMIN_EMAIL_DEF="admin@${DOMINIO#:*}"
	[[ "$DOMINIO" == ":80" ]] && SEED_ADMIN_EMAIL_DEF="admin@gpvdp.local"

	umask 077
	cat > "$ENV_FILE" <<EOF
# GPVDP ERP — configuración de producción. GENERADO EL $(date -u +%Y-%m-%dT%H:%M:%SZ).
#
# ⚠ ESTE ARCHIVO CONTIENE LAS CLAVES DEL SISTEMA.
#   · No se comparte, no se sube a ningún repositorio, no se manda por correo.
#   · Guardá una copia en un gestor de contraseñas: si se pierde JWT_SECRET todos tienen que
#     volver a ingresar; si se pierde POSTGRES_PASSWORD, el backend no puede abrir la base.
#   · Permisos 600 (solo root puede leerlo).

DOMINIO=${DOMINIO}
CORS_ORIGINS=${CORS_ORIGINS}

POSTGRES_DB=gpvdp
POSTGRES_USER=gpvdp
POSTGRES_PASSWORD=${POSTGRES_PASSWORD}

JWT_SECRET=${JWT_SECRET}
ACCESS_TTL=15m
# 7 días: si alguien pierde la computadora, la sesión caduca en una semana.
REFRESH_TTL=168h

SEED_ADMIN_EMAIL=${SEED_ADMIN_EMAIL_DEF}
SEED_ADMIN_PASSWORD=${ADMIN_PASS_NUEVA}
# El instalador lo enciende para el primer arranque y lo deja en false. NO lo enciendas a mano
# con la base ya cargada: revive catálogos desactivados y pisa la moneda de las cuentas.
SEED_ON_START=false

LIMITE_MEMORIA_DB=1g
EOF
	chmod 600 "$ENV_FILE"
	verde "  Secretos generados en $ENV_FILE (permisos 600)."
	# shellcheck disable=SC1090
	set -a; . "$ENV_FILE"; set +a
fi

titulo "5. Firewall"
if command -v ufw >/dev/null 2>&1; then
	# El orden importa: primero permitir SSH. Si se activa el firewall sin eso, la sesión
	# actual se corta y el servidor queda inalcanzable.
	ufw allow OpenSSH >/dev/null 2>&1 || ufw allow 22/tcp >/dev/null
	ufw allow 80/tcp  >/dev/null
	ufw allow 443/tcp >/dev/null
	ufw allow 443/udp >/dev/null
	if ufw status | grep -q "Status: active"; then
		verde "  Firewall ya activo; reglas verificadas (22, 80, 443)."
	else
		ufw --force enable >/dev/null
		verde "  Firewall activado: solo SSH (22) y web (80/443)."
	fi
	ama "  La base de datos NO está expuesta: no publica ningún puerto."
else
	ama "  ufw no está instalado; se omite el firewall. Cerrá los puertos en el panel del proveedor."
fi

titulo "6. Compilando y levantando (esto tarda varios minutos la primera vez)"
$COMPOSE build
$COMPOSE up -d db
verde "  Esperando a que la base esté lista…"
for i in $(seq 1 60); do
	if $COMPOSE exec -T db pg_isready -U "${POSTGRES_USER}" -d "${POSTGRES_DB}" >/dev/null 2>&1; then break; fi
	sleep 2
	(( i == 60 )) && morir "la base no arrancó; revisá: $COMPOSE logs db"
done
verde "  Base lista."

titulo "7. Datos iniciales"
# ¿Base nueva o ya poblada? Si ya hay empresas, no se siembra: sembrar sobre datos existentes
# revive catálogos que alguien desactivó a propósito.
YA_HAY="$($COMPOSE exec -T db psql -U "${POSTGRES_USER}" -d "${POSTGRES_DB}" -tAc \
	"SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name='empresa')" 2>/dev/null || echo f)"
HAY_EMPRESAS=f
if [[ "$YA_HAY" == "t" ]]; then
	HAY_EMPRESAS="$($COMPOSE exec -T db psql -U "${POSTGRES_USER}" -d "${POSTGRES_DB}" -tAc \
		"SELECT EXISTS (SELECT 1 FROM empresa)" 2>/dev/null || echo f)"
fi

if [[ "$HAY_EMPRESAS" == "t" ]]; then
	verde "  La base ya tiene datos: no se siembra nada (correcto)."
	$COMPOSE up -d backend web
else
	ama "  Base nueva: sembrando empresas, roles, permisos y el usuario administrador."
	SEED_ON_START=true $COMPOSE up -d backend
	for i in $(seq 1 60); do
		if $COMPOSE exec -T db psql -U "${POSTGRES_USER}" -d "${POSTGRES_DB}" -tAc \
			"SELECT EXISTS (SELECT 1 FROM empresa)" 2>/dev/null | grep -q t; then break; fi
		sleep 2
		(( i == 60 )) && morir "la siembra no terminó; revisá: $COMPOSE logs backend"
	done
	verde "  Datos iniciales creados."
	# Apagar la siembra y reiniciar SIN ella: que no vuelva a correr en cada reinicio.
	$COMPOSE up -d --force-recreate backend
	# Forzar el cambio de contraseña del administrador en el primer ingreso: la clave que
	# generó el instalador queda en un archivo, así que no debe seguir siendo la definitiva.
	$COMPOSE exec -T db psql -U "${POSTGRES_USER}" -d "${POSTGRES_DB}" -c \
		"UPDATE usuario SET debe_cambiar_password = true WHERE email = '${SEED_ADMIN_EMAIL}';" >/dev/null
	$COMPOSE up -d web
fi

titulo "8. Comprobando que responde"
OK=0
for i in $(seq 1 45); do
	if $COMPOSE exec -T backend wget -qO- http://127.0.0.1:8080/v1/healthz 2>/dev/null | grep -q ok; then OK=1; break; fi
	sleep 2
done
(( OK )) && verde "  El backend responde." || morir "el backend no responde; revisá: $COMPOSE logs backend"

# El esquema de la base. El backend aplica las migraciones al arrancar, y si una falla a medias
# golang-migrate marca la base como «sucia»: el sistema sigue respondiendo pero con el esquema
# incompleto, y una pantalla cualquiera falla más tarde sin que nadie relacione las dos cosas.
# Decir «instalado» sin mirar esto es exactamente la clase de silencio que no se puede permitir.
ESQUEMA="$($COMPOSE exec -T db psql -U "${POSTGRES_USER}" -d "${POSTGRES_DB}" -tAc \
	"SELECT version || CASE WHEN dirty THEN ' SUCIA' ELSE '' END FROM schema_migrations" 2>/dev/null || echo '?')"
if [[ "$ESQUEMA" == *SUCIA* ]]; then
	morir "una migración falló a medias y la base quedó marcada como sucia (versión ${ESQUEMA%% *}).
       El sistema NO está listo. Revisá: $COMPOSE logs backend"
elif [[ "$ESQUEMA" == "?" ]]; then
	morir "no se pudo leer la versión del esquema; revisá: $COMPOSE logs backend"
else
	verde "  Esquema de la base en la versión ${ESQUEMA} (sin marcas de error)."
fi

# Ahora por AFUERA, a través de Caddy. Que el backend responda por dentro no prueba que el sitio
# funcione: ya pasó una vez que un matcher del Caddyfile no acertara y el problema solo se veía
# desde el navegador. Se comprueban las DOS mitades: la aplicación compilada y la API.
titulo "8b. Comprobando el sitio (a través de Caddy)"
if (( SIN_DOMINIO )); then
	BASE_LOCAL="http://127.0.0.1"
	CURL_OPTS=(-fsS --max-time 20)
else
	# Con dominio, Caddy solo responde a ese nombre: se resuelve contra el propio servidor y se
	# acepta el certificado recién emitido (puede estar todavía en camino).
	BASE_LOCAL="https://${DOMINIO}"
	CURL_OPTS=(-fsSk --max-time 25 --resolve "${DOMINIO}:443:127.0.0.1")
	verde "  Caddy está pidiendo el certificado. Puede tardar hasta un minuto la primera vez."
	sleep 15
fi

SITIO_OK=0
for i in $(seq 1 20); do
	HTML="$(curl "${CURL_OPTS[@]}" "$BASE_LOCAL/" 2>/dev/null || true)"
	if grep -qi '<div id="root"' <<<"$HTML" 2>/dev/null; then SITIO_OK=1; break; fi
	sleep 3
done
if (( SITIO_OK )); then
	verde "  La aplicación se sirve correctamente."
	# El HTML tiene que apuntar a un archivo de /assets que EXISTA: un `dist` copiado a medias
	# devuelve el index sin su JavaScript y la pantalla queda en blanco, sin ningún error.
	ASSET="$(grep -o '/assets/[A-Za-z0-9._-]*\.js' <<<"$HTML" | head -1)"
	if [[ -n "$ASSET" ]] && curl "${CURL_OPTS[@]}" -o /dev/null "$BASE_LOCAL$ASSET" 2>/dev/null; then
		verde "  El programa de la aplicación ($ASSET) también se descarga."
	else
		ama "  ATENCIÓN: el index se sirve pero su JavaScript ($ASSET) no se pudo descargar."
		ama "  La pantalla se vería en blanco. Revisá: $COMPOSE logs web"
	fi
else
	ama "  El sitio todavía no responde por $BASE_LOCAL."
	if (( ! SIN_DOMINIO )); then
		ama "  Lo más común: el DNS de $DOMINIO no apunta acá todavía, así que el certificado no se"
		ama "  pudo emitir. Se resuelve solo cuando el DNS propague. Verificá con: $COMPOSE logs web"
	else
		ama "  Revisá: $COMPOSE logs web"
	fi
fi

# La API por el mismo camino que la usa el navegador.
if curl "${CURL_OPTS[@]}" "$BASE_LOCAL/v1/healthz" 2>/dev/null | grep -q ok; then
	verde "  La API responde por el mismo dominio (sin CORS de por medio)."
else
	ama "  La API no respondió a través de Caddy. Si el sitio tampoco, es el mismo motivo."
fi

titulo "9. Respaldo automático"
mkdir -p "$RAIZ/respaldos"
chmod 700 "$RAIZ/respaldos"
CRON_LINEA="0 2 * * * cd $RAIZ && bash deploy/respaldar.sh >> $RAIZ/respaldos/respaldo.log 2>&1"
if crontab -l 2>/dev/null | grep -qF "deploy/respaldar.sh"; then
	verde "  El respaldo diario ya estaba programado."
else
	( crontab -l 2>/dev/null; echo "$CRON_LINEA" ) | crontab -
	verde "  Respaldo diario programado a las 2:00 a.m. (se guardan 14 días)."
fi

# ─────────────────────────────── resumen ───────────────────────────────
if (( SIN_DOMINIO )); then URL="http://${IP_PUBLICA}"; else URL="https://${DOMINIO}"; fi
printf '\n\033[1m═══════════════════════════════════════════════════════\033[0m\n'
verde " GPVDP ERP instalado"
printf '\033[1m═══════════════════════════════════════════════════════\033[0m\n\n'
echo "  Dirección:   $URL"
echo "  Usuario:     ${SEED_ADMIN_EMAIL}"
if [[ -n "${ADMIN_PASS_NUEVA}" ]]; then
	echo ""
	ama "  Contraseña temporal: ${ADMIN_PASS_NUEVA}"
	ama "  ANOTALA AHORA. No se vuelve a mostrar, y el sistema te va a pedir cambiarla"
	ama "  en el primer ingreso. (También quedó en $ENV_FILE, que solo root puede leer.)"
else
	echo "  Contraseña:  la que ya tenías (esta instalación no la cambió)"
fi
cat <<RESUMEN

  Comandos útiles (desde $RAIZ):

    Ver el estado         docker compose -f deploy/docker-compose.prod.yml ps
    Ver los registros     docker compose -f deploy/docker-compose.prod.yml logs -f backend
    Respaldar ahora       bash deploy/respaldar.sh
    Publicar cambios      bash deploy/actualizar.sh
    Reiniciar             docker compose -f deploy/docker-compose.prod.yml restart

  Lo que sigue:
    1. Entrar y cambiar la contraseña del administrador.
    2. Crear un usuario por persona en Configuración → Usuarios (no compartir el admin).
    3. Copiar $ENV_FILE a un gestor de contraseñas.
    4. Probar UNA restauración del respaldo: uno que nunca se restauró no se sabe si sirve.

RESUMEN
