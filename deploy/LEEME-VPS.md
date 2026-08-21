# Instalar GPVDP ERP en un VPS

Guía para poner el sistema en un servidor de internet, con HTTPS y respaldos automáticos.

---

## Antes de empezar: qué servidor sirve

**Necesitás un VPS con acceso root**, no un hosting web compartido.

| Proveedor | Qué pedir | Sirve |
|---|---|---|
| **DigitalOcean** | Droplet, Ubuntu 24.04, **2 GB de RAM** o más | Sí |
| **GoDaddy** | **VPS** (no "Hosting Web" ni cPanel), Ubuntu | Sí, solo el VPS |
| Hetzner / Linode / Vultr | Ubuntu 22.04 o 24.04, 2 GB | Sí |
| GoDaddy "Hosting Web" / cPanel | — | **No**: no permite Docker |

**Mínimo real:** 2 GB de RAM, 2 núcleos, 25 GB de disco. Con 1 GB el instalador crea memoria de
intercambio y funciona, pero compilar tarda bastante más.

**Costo aproximado:** entre 12 y 24 dólares al mes en cualquiera de los tres primeros.

### El dominio

Para tener HTTPS hace falta un dominio (o un subdominio) apuntando al servidor:

1. En tu proveedor de dominios, creá un registro **A**: `erp` → la IP del servidor.
2. Esperá a que propague (unos minutos; con `ping erp.tudominio.com` se confirma).

Si todavía no tenés el dominio listo, se puede instalar en modo `--sin-dominio` para probar por IP,
**pero sin HTTPS las contraseñas viajan sin cifrar por internet**: solo para una prueba corta, nunca
para uso real con datos de la empresa.

---

## Instalación

### Armar el paquete para llevarlo al servidor

Lo más simple es generar UN archivo con todo adentro:

```bash
bash deploy/empaquetar.sh
```

Deja un `gpvdp-erp-AAAAMMDD.tar.gz` de poco más de 1 MB con el código completo del backend y del
frontend, los scripts y la documentación. Al descomprimirlo queda **una sola carpeta**,
`gpvdp-erp`, sin niveles repetidos.

El script **verifica el paquete antes de darlo por bueno**: cuenta los archivos de código a la
salida y los compara con el proyecto, comprueba que estén los archivos sin los que la instalación
no arranca, y —lo que más importa— falla si se colaron los **respaldos de la base** (movimientos
bancarios reales) o un `.env` con las claves. Un paquete de despliegue que lleva los datos de la
operación es una filtración, no una entrega.

Tres comandos, y queda instalado:

```bash
scp gpvdp-erp-AAAAMMDD.tar.gz root@LA-IP-DEL-SERVIDOR:/opt/
```

```bash
ssh root@LA-IP-DEL-SERVIDOR
```

```bash
cd /opt && tar -xzf gpvdp-erp-AAAAMMDD.tar.gz && cd gpvdp-erp && bash deploy/instalar-vps.sh erp.tudominio.com tucorreo@tudominio.com
```

Tarda entre 5 y 15 minutos (la mayor parte es compilar). Al final imprime la dirección y **una
contraseña temporal para el administrador: anotala, no se vuelve a mostrar.**

Sin dominio todavía (HTTP por la IP, solo para probar):

```bash
cd /opt/gpvdp-erp && bash deploy/instalar-vps.sh --sin-dominio
```

**De acá en adelante, el directorio de trabajo en el servidor es `/opt/gpvdp-erp`.**

### Alternativa: copiar la carpeta a mano

Si preferís no usar el paquete, se puede copiar el proyecto directo. Hay que copiar **el proyecto
completo**, no solo `deploy/`: el instalador compila desde `../backend` y `../frontend`, así que con
los scripts solos no hay nada que compilar.

La carpeta correcta es la que tiene **`docker-compose.yml` adentro**. Ese es el criterio, porque en
la instalación de desarrollo hay **dos carpetas anidadas con el mismo nombre**
(`GPVDP_ENTREGA/GPVDP_ENTREGA`) y la de afuera solo contiene a la de adentro: copiar la de afuera
deja el proyecto un nivel más abajo y ningún comando de esta guía funciona.

```bash
scp -r ".../Finance Group VDP/GPVDP_ENTREGA/GPVDP_ENTREGA" root@LA-IP-DEL-SERVIDOR:/opt/gpvdp-erp
```

Por este camino hay que excluir a mano lo que el paquete excluye solo: **`respaldos/`** (son volcados
con los movimientos bancarios reales) y cualquier `.env`.

### Qué hace el instalador

1. Comprueba el sistema, los permisos, la memoria y el disco.
2. Instala Docker desde el repositorio oficial (si falta).
3. Verifica que el dominio apunte a este servidor — así no se gastan intentos de certificado
   (Let's Encrypt permite 5 por semana y por dominio).
4. **Genera todos los secretos al azar** y los guarda en `.env` con permisos 600. Cada instalación
   tiene los suyos: no hay contraseñas de fábrica.
5. Activa el firewall dejando solo SSH y web. **La base de datos no queda expuesta.**
6. Compila y levanta el sistema.
7. Si la base está vacía, siembra los datos iniciales y **apaga la siembra** para que no vuelva a
   correr. Si ya tiene datos, no siembra nada.
8. Fuerza el cambio de contraseña del administrador en el primer ingreso.
9. **Comprueba tres cosas antes de decir «instalado»**, y si alguna falla lo dice en vez de callarla:
   - que el backend responda,
   - que la **versión del esquema** de la base esté aplicada y **sin marca de error** (si una
     migración falla a medias, el sistema sigue respondiendo con el esquema incompleto y el problema
     aparece días después en una pantalla cualquiera),
   - que el sitio se sirva **a través de Caddy** —el index y su JavaScript— y que la API responda por
     el mismo dominio. Que el backend funcione por dentro no prueba que el sitio abra: un error en el
     `Caddyfile` solo se ve desde el navegador.
10. Programa el respaldo diario a las 2 de la mañana.

**Se puede volver a correr sin miedo.** Detecta lo que ya está hecho y no lo repite; nunca borra datos.

---

## Después de instalar: cuatro cosas

1. **Entrar y cambiar la contraseña del administrador** (el sistema la pide sola).
2. **Crear un usuario por persona** en Configuración → Usuarios. No compartir la cuenta de
   administrador: si todos usan la misma, se pierde el rastro de quién hizo qué.
3. **Guardar una copia de `.env`** en un gestor de contraseñas. Si se pierde `JWT_SECRET`, todos
   tienen que volver a ingresar; si se pierde `POSTGRES_PASSWORD`, el sistema no puede abrir la base.
4. **Probar una restauración.** Un respaldo que nunca se restauró no se sabe si sirve. Ver abajo.

---

## Uso diario

Todo desde `/opt/gpvdp-erp`:

```bash
docker compose -f deploy/docker-compose.prod.yml ps
```

```bash
docker compose -f deploy/docker-compose.prod.yml logs -f backend
```

```bash
bash deploy/respaldar.sh
```

```bash
bash deploy/actualizar.sh
```

---

## Publicar cambios (el trabajo de los programadores)

Con el código actualizado en el servidor:

```bash
cd /opt/gpvdp-erp && bash deploy/actualizar.sh
```

El script, en este orden: **respalda primero**, compila las imágenes nuevas mientras el sistema sigue
en línea, recién entonces cambia los contenedores, y comprueba que responda. El corte de servicio son
los segundos del cambio, no toda la compilación.

Las migraciones de base de datos las aplica el backend al arrancar. El script muestra la versión
antes y después, y **se detiene si detecta la base en estado inconsistente**.

### Si una actualización sale mal

```bash
docker compose -f deploy/docker-compose.prod.yml logs --tail 80 backend
```

Para volver al código anterior: restaurá esa versión del código y corré `actualizar.sh` otra vez.
Si el problema es de datos, restaurá el respaldo que el script tomó antes de empezar (está en
`respaldos/`, con la fecha y hora en el nombre).

---

## Restaurar un respaldo

**Reemplaza los datos actuales por los del respaldo. No se puede deshacer.**

```bash
cd /opt/gpvdp-erp && docker compose -f deploy/docker-compose.prod.yml stop backend web
```

```bash
docker compose -f deploy/docker-compose.prod.yml cp respaldos/EL-ARCHIVO.dump db:/tmp/r.dump
```

```bash
docker compose -f deploy/docker-compose.prod.yml exec -T db pg_restore -U gpvdp -d gpvdp --clean --if-exists /tmp/r.dump
```

```bash
docker compose -f deploy/docker-compose.prod.yml up -d
```

> **Practicalo una vez** con un respaldo real, en un servidor de prueba, antes de necesitarlo de
> verdad. La primera restauración nunca debería ser durante una emergencia.

---

## Los respaldos están en el mismo servidor

El respaldo diario protege contra un error humano (alguien clasifica mal en masa), **no contra
perder el servidor**. Para eso hay que sacarlos afuera. La opción más simple es descargarlos a tu
computadora periódicamente:

```bash
scp root@LA-IP:/opt/gpvdp-erp/respaldos/*.dump ./respaldos-locales/
```

O configurar el envío automático a un almacenamiento del proveedor (Spaces en DigitalOcean, S3).

---

## Cómo está armado

```
        internet
           │  443 (HTTPS) y 80
      ┌────▼─────────────────────────┐
      │  Caddy                       │  certificado automático,
      │  · sirve la app compilada    │  se renueva solo
      │  · reenvía /v1 al backend    │
      └────┬─────────────────────────┘
           │  red interna de Docker
      ┌────▼──────┐      ┌───────────┐
      │  backend  ├─────►│    db     │  ningún puerto
      │  Go, :8080│      │ Postgres  │  hacia internet
      └───────────┘      └───────────┘
```

Tres decisiones que vale entender:

- **Un solo dominio para la pantalla y la API.** Eso elimina el CORS, y hace que la aplicación no
  tenga que conocer ninguna dirección: la deriva del sitio desde el que se abrió. Por eso los mismos
  archivos compilados sirven para cualquier dominio, y cambiar de dominio no obliga a recompilar.
- **Solo Caddy tiene puertos hacia internet.** El backend y la base se hablan por la red interna de
  Docker. Ni la base ni la API se pueden alcanzar desde afuera.
- **El frontend va compilado**, no con el servidor de desarrollo: más rápido, más liviano y no expone
  el código fuente.

### Archivos que sube el sistema, y los topes que hay

El ERP recibe archivos de verdad: los estados de cuenta de los bancos y el Excel de la clasificación
en bloque. Hay **tres topes, en tres capas distintas**, y cada uno da un mensaje diferente:

| Tamaño | Quién lo corta | Qué ve el usuario |
|---|---|---|
| hasta 16 MB | nadie, se procesa | el plan del archivo |
| 16 – 24 MB | la aplicación | «el archivo excede 16 MB: partilo por cuenta o por año» |
| más de 24 MB | Caddy, antes de llegar al backend | error 413 del navegador |

El tope de Caddy (`request_body max_size` en el `Caddyfile`) no es redundante: el importador de
estados de cuenta lee el archivo completo **sin tope propio**, así que sin ese límite una subida de
varios GB —por error o a propósito— llenaría el disco del servidor.

Los tiempos del proxy están en **300 segundos**: clasificar decenas de miles de filas puede tardar
minutos, y con el default de 60 s de muchos proxys la subida se cortaría a mitad de camino.

El backend tiene un tope de memoria de **2 GB** (`LIMITE_MEMORIA_BACKEND`). Es a propósito: si un
archivo gigante se lo come, muere el backend —y se reinicia solo— en vez de matar a la **base**, que
es lo único que no se puede perder.

### Cargar el histórico en producción

Dos pasos, en este orden, y el segundo no funciona sin el primero:

1. **Los movimientos**, en Bancos → Importador. Un archivo por cuenta; **puede traer varios meses**
   (no hay que subir mes por mes). Subir dos archivos que se solapan es seguro: la huella
   anti-duplicado descarta lo repetido.
2. **La clasificación ya hecha en Excel**, en el mismo Importador, panel «Traer la clasificación
   desde Excel». Bajá la plantilla, llená Concepto y Clasificación, subila. Previsualiza antes de
   escribir, y por defecto **no toca** lo que ya tiene partida.

---

## Probar el paquete de producción antes de publicarlo

Se puede levantar exactamente este mismo montaje en una computadora de trabajo, en puertos altos y
con una base separada que no toca la de desarrollo:

```bash
docker compose -p gpvdp_prueba_prod --env-file deploy/.env.prueba -f deploy/docker-compose.prod.yml -f deploy/docker-compose.prueba-local.yml up -d --build
```

Queda en `http://localhost:8090`. Para borrarlo todo, incluida su base de prueba:

```bash
docker compose -p gpvdp_prueba_prod -f deploy/docker-compose.prod.yml -f deploy/docker-compose.prueba-local.yml down -v
```

Hacé una copia de `deploy/.env.prueba` a partir del ejemplo y poné valores propios — ese archivo trae
credenciales de juguete y **no sirve para producción**.

---

## Mantener al día el paquete de los programadores

La carpeta `entrega-programadores/` tiene tres archivos GENERADOS: el esquema de la base y los
catálogos de rutas y de columnas. Se desfasan solos con cada cambio, y un `schema.sql` sin una
columna nueva **se corre sin dar error**: la aplicación falla después, en otra pantalla, sin que
nadie relacione las dos cosas.

Con el sistema levantado:

```bash
bash deploy/regenerar-entrega.sh
```

Regenera `schema.sql` y **verifica** los otros dos (no los reescribe: tienen agrupaciones y
explicaciones escritas a mano). Si falta documentar una ruta o una columna, lo dice y termina con
error, así se puede usar como control antes de entregarle el paquete a alguien.

---

## Lo que este montaje todavía NO tiene

Se dice para que nadie lo dé por hecho:

- **Réplica de la base.** Si el servidor se cae, el sistema queda fuera hasta que se restaure. Para
  algo así hace falta una base administrada (DigitalOcean Managed Database) o una réplica.
- **Monitoreo y alertas.** Nadie avisa si el sistema se cae. Lo más simple es un chequeo externo
  gratuito (UptimeRobot y similares) apuntando a `https://tudominio/v1/healthz`.
- **Límite de intentos de login.** No hay bloqueo por fuerza bruta. Con el sistema en internet, esto
  sube de prioridad: hoy la protección son contraseñas fuertes y que cada ingreso queda auditado.
- **Envío de correo real.** En desarrollo los correos van a un buzón de prueba (MailHog). Para que
  salgan de verdad hay que configurar un proveedor de SMTP.
- **Actualizaciones de seguridad del sistema operativo.** Conviene activar las automáticas:
  `apt install unattended-upgrades`.
