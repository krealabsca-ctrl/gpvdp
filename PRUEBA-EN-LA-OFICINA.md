# Probar GPVDP con los auxiliares — en la red de la oficina

> ## ⛔ ESTO ESTÁ DESACTIVADO HOY (17 de agosto de 2026)
>
> Por decisión del dueño, el sistema volvió a **uso local únicamente**: todos los puertos están
> atados a `127.0.0.1` y **nadie de la red puede entrar**, ni siquiera con el firewall abierto.
> Los datos quedaron intactos (se verificó movimiento por movimiento) y hay un respaldo en
> `respaldos/gpvdp_20260817_1055_antes_de_cerrar.dump`.
>
> **Para reabrirlo cuando quieras:** en `docker-compose.yml`, quitarle el prefijo `127.0.0.1:` a
> la línea de puertos del servicio **`frontend`** —solo a esa—, correr `docker compose up -d` y
> crear la regla de firewall del punto 1. Nada más: ya no hay direcciones que editar.
>
> El resto de este documento sigue siendo válido y es la guía para ese momento. Ojo con una cosa:
> **la dirección IP que aparece más abajo (`192.168.1.171`) puede haber cambiado** — el router la
> reasigna. Verificala con el comando del punto 2 antes de compartirla.


El sistema **no está publicado en internet** y no va a estarlo con esta configuración. Corre en tu
computadora y los auxiliares lo alcanzan por la red interna, igual que se alcanza una impresora
compartida. Si alguien fuera de la oficina intentara entrar, no llega: no hay ninguna puerta abierta
hacia afuera.

Dirección del sistema para tus compañeros:

```
http://192.168.1.171:5173
```

---

## Lo que te toca hacer a vos (una sola vez, 5 minutos)

### 1. Abrir el paso en el firewall de Windows

Hoy Windows bloquea las conexiones de otras computadoras. Hay que permitir **un solo puerto**, y
solo para la red de la oficina. Abrí PowerShell **como administrador** (clic derecho en el botón de
Inicio → «Terminal (Administrador)») y pegá esto:

```bash
New-NetFirewallRule -DisplayName "GPVDP web" -Direction Inbound -Protocol TCP -LocalPort 5173 -RemoteAddress 192.168.1.0/24 -Profile Private -Action Allow
```

El `-RemoteAddress 192.168.1.0/24` es lo importante: significa «solo computadoras de esta oficina».
Si algún día querés cerrar el acceso:

```bash
Remove-NetFirewallRule -DisplayName "GPVDP web"
```

> Antes hacían falta **dos** reglas (5173 y 8080). Ya no: la API viaja por dentro del mismo puerto
> del sistema, así que hay un puerto menos abierto en la red.

### 2. Fijar la dirección de tu computadora

**Esto ya pasó una vez:** tu máquina tenía `192.168.1.115` y el router se la cambió a
`192.168.1.171`. Ahora el sistema aguanta ese cambio sin romperse por dentro, pero **la dirección
que le pasás a tus compañeros sí cambia**, y si no se la actualizás no van a poder entrar.

La forma correcta es pedirle al router que reserve la dirección siempre para esta computadora
(«reserva DHCP» o «IP fija», según la marca). Si no tenés acceso al router, avisale a quien lo
administre.

Para saber cuál es tu dirección en cualquier momento, en PowerShell:

```bash
(Get-NetIPAddress -AddressFamily IPv4 | Where-Object InterfaceAlias -like "Wi-Fi*").IPAddress
```

> Buena noticia: **ya no hay que editar ningún archivo cuando la IP cambia.** El sistema se adapta
> solo a la dirección por la que cada persona entra. Lo único que hay que actualizar es el enlace
> que les compartís.

### 3. Cambiar la contraseña del administrador

La contraseña de administrador estaba escrita en el README y en el archivo de arranque; ya la quité
de ahí, pero **la contraseña sigue siendo la misma hasta que la cambies**. Entrá al sistema con tu
usuario admin y cambiala desde la pantalla de cambio de contraseña. Nadie más debe usar esa cuenta:
cada auxiliar tiene la suya.

---

## Crear la cuenta de cada auxiliar

En el sistema: **Configuración → Usuarios → + Nuevo usuario**.

- **Rol:** `Auxiliar Financiero` — verificá a ojo que diga eso antes de guardar. El desplegable
  también ofrece «Administrador», y ese le daría acceso a todo.
- **Contraseña temporal:** poné una distinta para cada persona. El sistema **obliga a cambiarla** en
  el primer ingreso, así que no queda en tus manos.
- Pasale la contraseña por un canal aparte (en persona o por mensaje), no en el mismo correo donde
  le mandás la dirección.

El alta se hace **sobre la empresa que tengas activa** en ese momento. Para esta prueba: **Valle de
Paz**.

### Qué puede hacer ese rol (ya verificado)

| Sí puede | No puede |
|---|---|
| Ver y filtrar la Bandeja de CxP | **Aprobar facturas** (ni de a una ni en lote) |
| Clasificar el gasto | **Programar y pagar** |
| **Marcar facturas como revisadas** | Ver la cartera abierta (la deuda total) |
| Registrar proveedores e importar facturación | Tocar los umbrales de validación |
| Caja chica: vales y reposiciones | Cobros y cartera de CxC |
| Empleados, ausencias y corridas de nómina | Parámetros de nómina y finiquitos |
| | Crear usuarios o cambiar permisos |

La frontera importante es esa: **registran y revisan, pero no firman ni pagan**.

---

## Respaldo: la regla que no se salta

Van a probar sobre los datos reales. La auditoría no guarda el valor anterior de cada cambio, así
que **si alguien clasifica mal 200 facturas, la única vuelta atrás es el respaldo**.

Doble clic en **`Respaldar-GPVDP.bat`**:

- **Antes** de abrirles el sistema el primer día.
- **Al final de cada día** de prueba.

Se puede correr con gente trabajando adentro. Los respaldos quedan en la carpeta `respaldos\`.
Copiá al menos uno a un disco externo: un respaldo que vive solo en la computadora no protege
contra que se dañe la computadora.

Ya hay dos respaldos hechos de hoy, tomados antes de cualquier cambio.

---

## Reglas de convivencia mientras dure la prueba

- **No corras `Levantar-GPVDP.bat` con gente adentro.** Ese archivo reconstruye el sistema y corta a
  todos. Si solo necesitás reiniciar, usá `docker compose up -d` (sin reconstruir).
- **Tu computadora tiene que estar encendida** y con Docker corriendo. Si se apaga o se suspende,
  todos pierden el sistema. Conviene desactivar la suspensión automática mientras dure la prueba.
- **Nunca agregues `-v`** a un `docker compose down`. Esa letra borra la base de datos entera.
- Si alguien reporta «no me carga», lo primero es confirmar que tu computadora esté prendida y que
  tu dirección siga siendo la que les pasaste (`ipconfig`, o el comando del punto 2). Que el router
  te la cambie es la causa más probable, y ya pasó una vez.

---

## Lo que quedó cerrado a propósito

Estos tres servicios **ya no se ven desde la red**, solo desde tu computadora:

| Servicio | Por qué se cerró |
|---|---|
| Base de datos (5432) | Acceso directo a todos los datos, sin permisos ni registro de quién hizo qué |
| Adminer (8081) | Administrador de la base: permite borrar cualquier cosa de un clic |
| MailHog (8025) | Muestra los correos que emite el sistema |

Vos los seguís usando normalmente en `http://localhost:8081` y `http://localhost:8025`.

---

## Lo que NO conviene hacer todavía

- **Abrir puertos en el router** («port forwarding») para entrar desde afuera. Eso sí sería publicar
  el sistema en internet, y hoy el tráfico viaja sin cifrar.
- Usar servicios tipo ngrok o túneles públicos sin autenticación.
- Compartir la cuenta de administrador entre varias personas: se pierde el rastro de quién hizo qué,
  que es justamente lo que sirve para corregir un error de la prueba.

Cuando la prueba madure y quieras acceso desde fuera de la oficina, el camino es una red privada
(tipo Tailscale) o un servidor con certificado — pero eso es otra conversación, y no hace falta para
esta etapa.

---

## Una cosa que conviene saber

Todo el tráfico dentro de la oficina va **sin cifrar** (HTTP, no HTTPS). En una red interna de
confianza es aceptable para una prueba, pero implica dos cosas concretas:

1. Que las contraseñas de los auxiliares sean **únicas de este sistema** — no la misma del correo ni
   la del banco.
2. Que la prueba corra sobre la red de la oficina, no sobre una red de visitas o compartida.
