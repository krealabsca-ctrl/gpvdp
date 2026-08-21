# GPVDP ERP — Manual técnico de entrega

Documento para el equipo de programación que va a recibir, poner en producción y mantener el
sistema. Todos los números de este manual salieron de medir el sistema real, no de estimaciones.

**Fecha de corte:** 14 de agosto de 2026 · **Migración aplicada:** 0062 · **73 tablas**

---

## 1. Qué es este sistema

ERP financiero **multiempresa** para Grupo Valle de Paz (Valle de Paz, Coopeprofa, Memorial Pets;
diseñado para agregar más). Es el **sistema de registro**: no hay una contabilidad externa detrás,
lo que está acá es el dato bueno.

Cinco módulos, todos en operación:

| Módulo | Qué resuelve | Endpoints | Tablas |
|---|---|---|---|
| **Bancos** | Importar estados de cuenta de 7 bancos de Costa Rica, clasificar movimientos con un motor que aprende, tipo de cambio, conciliación bancaria, cierre de período | 69 | 18 |
| **CxP** | Cuentas por pagar de punta a punta: recepción, clasificación, validación por riesgo, matriz de firmas, lotes de pago, conciliación, caja chica, anticipos | 69 | 11 |
| **CxC** | Cartera de contratos funerarios: cargos por período, cobros, planillas de asociaciones, cola de cobro por valor esperado, arreglos de pago, suspensión por mora | 42 | 23 |
| **RRHH / Nómina** | Planilla quincenal y mensual conforme a la ley de Costa Rica: cargas CCSS, renta, incapacidades, vacaciones, finiquitos, archivo SINPE, provisiones | 43 | 11 |
| **Configuración** | Usuarios, roles, matriz de permisos por empresa, plantillas de correo | 16 | 10 |

**Tamaño:** 40.805 líneas de Go (más 9.329 de pruebas) y 35.360 de TypeScript.

### Lo que este sistema NO hace

Decirlo evita que alguien lo busque:

- **No emite factura electrónica** ni firma XML para Hacienda. En CxP *recibe* comprobantes de
  proveedores; en CxC no emite documentos electrónicos.
- **No ejecuta pagos.** Genera el archivo de carga masiva (SINPE) que una persona sube al banco.
- **No tiene contabilidad de partida doble** ni libro mayor. Clasifica por Concepto › Clasificación,
  que es el eje analítico que usa la empresa.

---

## 2. Stack y versiones

| Capa | Tecnología | Versión |
|---|---|---|
| Backend | Go + Gin | Go 1.26 |
| Base de datos | PostgreSQL | **16** (requerido) |
| Driver | `pgx/v5` | — |
| Migraciones | `golang-migrate` | embebidas en el binario |
| Dinero | `shopspring/decimal` | — |
| Excel | `excelize/v2` | — |
| Logs | `zap` | estructurados |
| Frontend | Vite + React + TypeScript | React 18, TS estricto |
| Estilos | Tailwind CSS | tokens semánticos |
| Estado de servidor | TanStack Query | — |
| Gráficos | Recharts | — |
| Infra local | Docker Compose | — |

**Extensión de PostgreSQL requerida:** `pgcrypto` (para `gen_random_uuid()`). El `schema.sql` la crea.

---

## 3. Qué hay en esta carpeta

| Archivo | Para qué sirve |
|---|---|
| **`schema.sql`** | El esquema completo: 73 tablas, 231 índices, 189 claves foráneas. Se corre sobre una base vacía y queda toda la estructura. **Probado**: recrea la base real con exactamente las mismas 73 tablas, 789 columnas, 231 índices y 189 FKs. |
| **`ENDPOINTS.md`** | Las 247 rutas de la API con su método y el permiso que exige cada una. Generado del router, no escrito a mano. |
| **`MANUAL-TECNICO.md`** | Este documento. |

El código fuente está en `backend/` y `frontend/`, un nivel arriba.

---

## 4. Levantar el sistema

### Ambiente de desarrollo (lo que usamos hoy)

```bash
docker compose up -d --build
```

Levanta Postgres, el backend (`:8080`), el frontend (`:5173`), Adminer (`:8081`) y MailHog (`:8025`).
El backend **aplica las migraciones solo al arrancar**, así que no hay paso manual de esquema.

### Primer arranque contra una base nueva

Dos caminos, y conviene entender la diferencia:

**Camino A — dejar que el backend construya el esquema** (recomendado para ambientes reales):

1. Base vacía y `DATABASE_URL` apuntando a ella.
2. Arrancar el backend: aplica las 62 migraciones en orden y registra en `schema_migrations`.
3. Poner `SEED_ON_START=true` **solo en ese primer arranque** para sembrar empresas, roles,
   permisos y el usuario administrador. **Apagarlo después** (ver §9).

**Camino B — correr `schema.sql`** (para inspeccionar o comparar):

```bash
createdb -U postgres gpvdp
psql -U postgres -d gpvdp -f schema.sql
```

Deja la estructura completa pero **sin la tabla `schema_migrations` poblada**, así que el backend
intentará aplicar las 62 migraciones sobre tablas que ya existen y fallará. Si se usa este camino,
hay que marcar la versión a mano:

```sql
INSERT INTO schema_migrations (version, dirty) VALUES (62, false);
```

> Por eso el Camino A es el bueno para producción: deja la base y el registro de migraciones
> consistentes entre sí, sin intervención manual.

### Variables de entorno

| Variable | Qué hace | Ojo |
|---|---|---|
| `DATABASE_URL` | Conexión a Postgres | — |
| `JWT_SECRET` | Firma de los tokens | **Sin valor por defecto: el backend no arranca si falta.** Cambiarlo invalida todas las sesiones. |
| `ACCESS_TTL` | Vida del access token | hoy `15m` |
| `REFRESH_TTL` | Vida del refresh token | hoy `720h` (30 días) — **revisar para producción** |
| `CORS_ORIGINS` | Orígenes permitidos, separados por coma | **Igualdad exacta**, sin comodines. Un origen que no esté acá recibe 200 sin la cabecera y el navegador descarta la respuesta. |
| `SEED_ON_START` | Siembra catálogos al arrancar | Ver §9: debe quedar en `false` una vez sembrado |
| `SEED_ADMIN_EMAIL` / `_PASSWORD` | Administrador inicial | Solo se usan si `SEED_ON_START=true` |
| `CIERRE_PERIODO_BLOQUEANTE` | Exige 100 % clasificado para cerrar mes | `true` por decisión del negocio |
| `APP_ENV` | `development` / `production` | En producción, Gin pasa a modo release |

---

## 5. Modelo de datos

### 5.1 Convenciones de tipos — respeten estas

Medido sobre las 789 columnas reales:

| Para | Tipo | Columnas | Por qué |
|---|---|---|---|
| Identificadores | `uuid` con `DEFAULT gen_random_uuid()` | 267 | Sin secuencias: permite generar el id antes de insertar y no filtra volumen de negocio |
| Texto | **`text`** (sin límite) | 174 | No hay un solo `varchar(n)` en el esquema. En PostgreSQL `text` no es más lento y evita migraciones por «se quedó corto el campo» |
| Fecha con hora | `timestamptz` | 99 | **Siempre con zona.** Nunca `timestamp` pelado |
| Banderas | `boolean` | 59 | — |
| **Dinero** | **`numeric(14,2)`** | 50 | Montos mayores: `numeric(16,2)` (19 col) y `numeric(18,2)` (8 col) |
| Cantidades | `integer` / `smallint` / `bigint` | 52 | — |
| Fecha sin hora | `date` | 25 | Fechas de emisión, vencimiento, período |
| Estructuras | `jsonb` | 8 | Solo donde la forma es genuinamente variable |
| Tipo de cambio | `numeric(14,4)` | 5 | Cuatro decimales: el TC del BCCR los usa |
| Porcentajes | `numeric(5,2)`, `numeric(6,3)` | 9 | — |
| Archivos | `bytea` | 2 | El .xlsx original de cada importación bancaria |

**Regla que no se negocia: dinero nunca en `float`, `double precision` ni `real`.** Hoy el esquema
no tiene ni una columna de punto flotante, y en Go el dinero viaja como `decimal.Decimal` y sale al
JSON como **string**, no como número — para que ningún cliente lo redondee al parsear.

### 5.2 Las 73 tablas, por módulo

Casi todas llevan `empresa_id uuid NOT NULL` (ver §6.1).

#### Núcleo, seguridad y auditoría (10)

| Tabla | Col. | Qué guarda |
|---|---|---|
| `empresa` | 6 | Las empresas del grupo |
| `usuario` | 8 | Personas. `password_hash` bcrypt, `debe_cambiar_password` fuerza el cambio al primer ingreso |
| `usuario_empresa_rol` | 5 | Qué rol tiene cada persona **en cada empresa** |
| `rol` | 6 | Roles. **Son globales** (`empresa_id IS NULL`) salvo los creados a medida |
| `permiso` | 7 | Catálogo de 55 permisos. `critico` marca los sensibles |
| `rol_permiso` | 4 | La matriz permiso × rol × **empresa**. Editable en caliente |
| `sesion` | 6 | Refresh tokens vigentes |
| `auditoria_evento` | 9 | Append-only: quién, qué, cuándo. **Ver la limitación en §11** |
| `plantilla_correo` | 7 | Textos de los correos, configurables por empresa |
| `schema_migrations` | 2 | La lleva golang-migrate. **No tocar a mano** salvo el caso de §4 |

#### Bancos (18)

| Tabla | Col. | Qué guarda |
|---|---|---|
| `banco` | 5 | Bancos (13 registrados) |
| `cuenta_bancaria` | 8 | Cuentas con IBAN y moneda (20 registradas) |
| `concepto` | 8 | Primer nivel del catálogo de gasto/ingreso. `naturaleza` define si suma al EBITDA (§11) |
| `clasificacion` | 8 | Segundo nivel, cuelga del concepto (284 registradas) |
| `subclasificacion` | 6 | Tercer nivel, opcional |
| `movimiento_bancario` | 26 | El corazón: 14.181 filas. `natural_key` + `indice_ocurrencia` es el anti-duplicado |
| `importacion` | 10 | Cada archivo cargado, **con el .xlsx original en `bytea`** para reproducir fallas |
| `regla_clasificacion` | 10 | El motor que aprende: patrón → clasificación |
| `palabra_clave` | 4 | Palabras de cada regla |
| `proveedor_gasto` | 8 | Memoria: qué gasto suele tener cada proveedor |
| `tipo_cambio_mes` | 8 | TC del mes; se **congela** y queda inmutable |
| `tipo_cambio_cotizacion` | 6 | Cotizaciones diarias |
| `bccr_sync_log` | 8 | Bitácora de la sincronización con el BCCR |
| `periodo_cierre` | 7 | Meses cerrados |
| `proyeccion_escenario` | 13 | Escenarios de flujo de caja |
| `saldo_cuenta_diario` | 11 | Saldo por cuenta y día, con cuadre derivado |
| `acta_conciliacion` | 12 | Acta mensual por cuenta; firmarla habilita el cierre |
| `partida_conciliacion` | 14 | Partidas en tránsito del acta |

#### Cuentas por Pagar (11)

| Tabla | Col. | Qué guarda |
|---|---|---|
| `proveedor` | 20 | 649 proveedores, con IBAN, retención y gasto predeterminado |
| `documento_cxp` | **41** | La factura y todo su ciclo: 4.542 filas. La tabla más ancha del sistema |
| `documento_cxp_aprobacion` | 6 | Cada firma: la matriz por monto puede pedir varias |
| `anticipo_aplicacion` | 10 | Anticipos neteados contra la factura final |
| `departamento` | 9 | Áreas / centros de costo |
| `departamento_validador` | 4 | Quién valida cada área (titular y suplente) |
| `lote_pago` | 7 | Lotes que van al banco |
| `comprobante_pago` | 8 | Comprobantes enviados al proveedor |
| `caja_chica_fondo` | 11 | Fondos fijos con custodio, umbral y límite por vale |
| `caja_chica_vale` | 14 | Vales; los estados se derivan por SQL, sin triggers |
| `cxp_parametro` | 6 | Umbrales de la validación por riesgo (§11) |

#### Cuentas por Cobrar (23)

| Tabla | Col. | Qué guarda |
|---|---|---|
| `contrato_cxc` | 28 | El contrato funerario: el eje de todo el módulo |
| `cargo_cxc` | 13 | Cargos por período (partidas abiertas) |
| `cobro_cxc` | 25 | Cobros recibidos |
| `cobro_aplicacion` | 7 | Cómo se aplicó cada cobro (FIFO, lo más viejo primero) |
| `nota_credito_cxc` | 14 | Condonar o corregir **sin editar el cargo original** |
| `nota_credito_aplicacion` | 7 | Aplicación de la nota |
| `arreglo_pago_cxc` | 23 | Arreglos 1-3-6-9 que **no reescriben** los cargos |
| `arreglo_cuota_cxc` | 6 | Cuotas del arreglo |
| `gestion_cxc` | 11 | Llamadas y mensajes de cobro |
| `promesa_pago_cxc` | 7 | Promesas, con cumplimiento derivado |
| `cxc_suspension` | 12 | Suspensión por mora (18 meses o su equivalencia) |
| `cxc_asociacion` | 9 | Asociaciones solidaristas |
| `cxc_planilla` | 9 | Planillas de deducción |
| `cxc_planilla_movimiento` | 5 | Detalle, conciliado contra el depósito en Bancos |
| `cxc_importacion` | 14 | Cargas desde el sistema de origen |
| `cxc_parametro` | 6 | 42 parámetros del módulo |
| `cxc_tramo` | 11 | Tramos de mora con probabilidad de recuperación |
| `cxc_sede` | 9 | Sedes: **la frontera de datos del cobrador** |
| `cxc_usuario_sede` | 4 | Qué sedes ve cada usuario |
| `cxc_modalidad` | 7 | Mensual, quincenal, semanal… |
| `cxc_forma_pago` | 8 | Con su factor de recuperación |
| `cxc_canal_gestion` | 6 | Teléfono, WhatsApp, visita… |
| `cxc_resultado_gestion` | 10 | Resultados posibles de una gestión |

> **Nota de capacidad:** este módulo se probó con **70.000 contratos** cargados. Los índices y la
> consulta de la cola de cobro están afinados para ese volumen.

#### RRHH / Nómina (11)

| Tabla | Col. | Qué guarda |
|---|---|---|
| `empleado` | 18 | Ficha, salario, jornada (mensual o quincenal) |
| `deduccion_empleado` | 12 | Deducciones recurrentes con saldo y número de cuotas |
| `concepto_nomina` | 11 | Conceptos con banderas de afectación (CCSS / renta / aguinaldo) |
| `nomina_parametros` | 18 | Cargas sociales y tramos de renta, **versionados por año** |
| `corrida_nomina` | 22 | La corrida del período |
| `corrida_linea` | 26 | Una línea por trabajador con todo el desglose |
| `corrida_novedad` | 7 | Horas extra y novedades del período |
| `incapacidad` | 11 | CCSS / INS con el subsidio de ley |
| `vacacion` | 9 | Disfrute; el saldo se **deriva**, no se almacena |
| `finiquito` | 28 | Liquidación de cese conforme al Código de Trabajo |
| `nomina_archivo_pago` | 8 | Archivos SINPE generados |

---

## 6. Las reglas que no se pueden romper

Si se rompe una de estas seis, el sistema deja de ser confiable como registro. Están sostenidas por
código y por pruebas.

### 6.1 Aislamiento por empresa

**Toda** consulta filtra por `empresa_id`, y ese valor sale **del token** (lo inyecta el middleware
de tenant), nunca del cuerpo ni del query string. Un repositorio que no reciba y aplique `empresa_id`
es un bug de seguridad, no un descuido de estilo.

```go
// Correcto: empresa_id viene del contexto
func (r *pgRepository) Listar(ctx context.Context, empresaID string, ...) {
    // WHERE d.empresa_id = $1::uuid
}
```

### 6.2 Dinero nunca en punto flotante

`numeric` en la base → `decimal.Decimal` en Go → **string** en el JSON. Un `float64` en la ruta del
dinero introduce errores de centavos que después nadie encuentra.

### 6.3 Nada de borrado físico en tablas financieras

Se desactiva (`activo = false`) o se anula con un documento que revierte. Un `DELETE` sobre una tabla
financiera destruye la trazabilidad. Para corregir catálogos hay operaciones específicas: eliminar
solo si nada cuelga, desactivar si algo cuelga, y **fusionar**, que mueve movimientos, reglas,
facturas y vales al destino.

### 6.4 El pasado no se reescribe

Los hechos derivados del catálogo se **congelan** cuando el documento sale del tramo abierto. Ejemplo
vivo: si una factura se aprobó porque su proveedor estaba marcado «de Contabilidad», esa razón queda
sellada en la factura — desmarcar el proveedor mañana no puede borrar por qué se aprobó ayer. Lo
mismo con el veredicto de la validación por riesgo (§11) y con el tipo de cambio del mes.

### 6.5 Una sola expresión SQL por concepto compartido

Cuando dos pantallas hablan del mismo hecho, comparten **la misma constante SQL**. No dos consultas
equivalentes: la misma.

```go
// backend/internal/cxp/fase_bandeja.go
const faseBandejaSQL = `CASE ... END`   // lo usan el resumen del encabezado Y el filtro del listado
```

Esto no es elegancia: cuando estaban escritas por separado, discreparon, y el encabezado contaba una
factura en dos pestañas a la vez. Otros casos: `condicionesMovimientos`, `contabilidadOrigenSQL`,
`sqlIngresoNeto` / `sqlGastoNeto`.

### 6.6 El día es el de Costa Rica, en las dos capas

El contenedor corre en UTC. «Hoy» en UTC−6 no es «hoy» en UTC seis horas al día, y de eso depende si
una factura está vencida.

- SQL: `(now() AT TIME ZONE 'America/Costa_Rica')::date`
- Go: `hoyCR()` / `AhoraCR()`

Nunca `time.Now()` pelado para un dato que el usuario va a leer como fecha.

---

## 7. Arquitectura

### Backend: handler → service → repository

Dependencias hacia adentro, sin excepciones:

```
handler (Gin)     parsea y valida el request, mapea a HTTP.  CERO SQL, CERO negocio.
    ↓
service           TODA la lógica y las reglas. No conoce Gin ni HTTP.
    ↓
repository        interfaz + implementación pgx. Devuelve tipos de dominio, no filas.
```

Paquetes en `backend/internal/`:

| Paquete | Contenido |
|---|---|
| `server` | Router, middleware, CORS. **`router.go` es el índice de todo el sistema**: 247 rutas con su permiso |
| `auth` | Login, JWT, refresh, cambio de contraseña |
| `tenant` | Middleware de empresa y verificación de permisos |
| `rbac` | Catálogo de permisos, matriz por empresa, usuarios |
| `bancos`, `cxp`, `cxc`, `nomina` | Los cuatro dominios |
| `plantillas` | Plantillas de correo |
| `shared` | Auditoría y utilidades comunes |
| `config`, `database`, `logging`, `httpx` | Infraestructura |
| `seed` | Siembra inicial (ver §9) |

**Errores:** tipados por dominio en `errors.go`, envueltos con `%w`, comparados con `errors.Is`. El
handler los traduce a HTTP: 400 validación, 401/403 permisos, 404 no encontrado, 409 conflicto,
422 regla de negocio, 500 interno. Nunca se filtra un error interno crudo al cliente.

### Frontend

```
frontend/src/
  api/          cliente tipado por módulo (cxp.ts, bancos.ts…) + queryKeys.ts
  features/     un directorio por módulo: pages/, components/, hooks.ts
  components/ui capa de componentes compartidos
  routes/       rutas y el guard de permisos
  lib/          formato de moneda y fecha, errores, utilidades
```

Dos cosas que muerden si no se saben:

**1. El orden de las claves de caché importa.** TanStack Query invalida **por prefijo**, así que la
empresa tiene que ir en la posición que permita invalidar el grupo entero:

```ts
// api/queryKeys.ts — respetar la forma existente de cada módulo
bancos: { dashboard: (empresaId) => ["bancos", "dashboard", empresaId] }
```

Una invalidación mal armada no falla: **miente**. Se verifica mutando y leyendo en la misma pantalla.

**2. Tokens semánticos de Tailwind, no colores literales.** `surface`, `surface-raised`,
`content`, `content-muted`, `border`, `accent`, `positivo`, `negativo`, `pendiente`. Así el tema
oscuro y el de cada empresa funcionan sin tocar cada componente.

---

## 8. Permisos (RBAC)

**55 permisos** en `backend/internal/rbac/catalogo.go`, que es la fuente de verdad. La matriz
permiso × rol × empresa se edita desde la interfaz y surte efecto casi en vivo.

Roles base: `ADMIN` (bypass total por diseño, para que nadie se auto-bloquee),
`DIRECTOR_FINANCIERO`, `SUPERVISOR_FINANCIERO`, `AUXILIAR_FINANCIERO`, `GERENCIA_GENERAL`,
`AUDITOR_INTERNO`. Se pueden crear roles a medida.

### Tres trampas verificadas en producción

1. **Los roles son globales, la matriz es por empresa.** `rol.empresa_id IS NULL` para los roles
   base, pero `rol_permiso` lleva `empresa_id`. Una migración que agregue un permiso **debe**
   hacer `CROSS JOIN empresa` y aceptar `(r.empresa_id IS NULL OR r.empresa_id = e.id)`. Sin eso
   falla por violación de NOT NULL.

2. **Un rol que no está en `rbac.MatrizDefault` queda congelado.** Una empresa nueva nace con cero
   permisos para ese rol y «aplicar permisos faltantes» nunca se los da, porque no sabe cuáles son.

3. **Dos capas que se pueden contradecir.** En CxP, las acciones masivas verifican el rol dentro de
   `masivo.go`, además del permiso que ya verifica el router. Si se concede un permiso desde la
   interfaz y no se alinea esa lista, el sistema deja hacer la acción de a una y la niega en lote —
   la misma operación, dos respuestas. Nos pasó con `cxp.revisar`.

Después de tocar `MatrizDefault`: reiniciar el backend y pulsar **«Aplicar permisos nuevos»** en
`/usuarios` **una vez estando en cada empresa**. Los permisos nuevos no bajan solos a una base que
ya existe.

Hay un test que afirma el conteo exacto de permisos: agregar uno y no actualizarlo pone la suite en
rojo. Es a propósito — obliga a que agregar un permiso sea una decisión consciente.

---

## 9. Cómo se cambia el esquema

**Esta es la sección que importa para el trabajo del día a día.**

La fuente de verdad son las migraciones en `backend/migrations/`, embebidas en el binario
(`embed.go`) y aplicadas por `golang-migrate` al arrancar.

### Agregar un cambio

1. Crear **dos** archivos con el número siguiente:

```
backend/migrations/0063_lo_que_hace.up.sql
backend/migrations/0063_lo_que_hace.down.sql
```

2. El `.up.sql` hace el cambio; el `.down.sql` lo revierte. **Reversible siempre.**
3. Reconstruir el backend: `docker compose up -d --build backend`. Las migraciones están **dentro de
   la imagen**, así que un `up -d` sin `--build` no las incluye — es la confusión más frecuente.
4. Verificar:

```sql
SELECT version, dirty FROM schema_migrations;
```

`dirty = true` significa que una migración falló a medias: hay que arreglar el SQL, limpiar la
marca y volver a aplicar. **No se avanza con la base sucia.**

### Reglas para escribir migraciones

- **Idempotentes** donde se pueda: `IF NOT EXISTS`, `ON CONFLICT DO NOTHING`.
- **Nunca editar una migración ya aplicada** en cualquier ambiente. Se corrige con una nueva.
- Si toca permisos, releer §8 (el `CROSS JOIN empresa`).
- Si toca datos, decir en un comentario **qué se recalcula y qué se deja intacto**. La regla 6.4
  vale también acá: una migración que reescribe el pasado borra la explicación de por qué algo pasó.
- Los `COMMENT ON COLUMN` se usan y valen: aparecen en la base y explican columnas no obvias.

### El seed: entender qué hace antes de encenderlo

`SEED_ON_START=true` siembra empresas, roles, permisos, bancos, cuentas y el administrador. Es para
**arrancar de cero**.

**Hoy está en `false` a propósito.** Con la base ya poblada, cada arranque reactivaba bancos
desactivados y pisaba la moneda de las cuentas. La causa: su `ON CONFLICT` compara por **nombre**, y
el nombre es un campo que el usuario puede cambiar. Un seed «idempotente» no lo es si compara por
algo mutable — tiene que preguntar «¿ya hay datos?».

Las migraciones y la siembra de permisos (`rbacSvc.EnsureDefaults`) corren **siempre**, aparte del
seed. Apagarlo no pierde nada.

---

## 10. Trabajo en equipo: lo que falta montar

**El proyecto no tiene control de versiones.** No hay repositorio git. Esto es lo primero que hay
que resolver, porque sin eso no existe forma ordenada de que varias personas incorporen cambios —
que es justamente lo que se busca.

Lo que se pierde hoy sin git, con un caso real: al correr una herramienta de formateo se reescribió
un archivo entero por accidente y no había con qué comparar; hubo que recuperarlo de la copia que
vivía dentro del contenedor.

Al inicializar el repositorio, el `.gitignore` **tiene que** excluir desde el primer commit:

```gitignore
.env
.env.local
respaldos/          # dumps de la base: datos financieros reales
*.dump
*.sql.gz
node_modules/
dist/
frontend/public/maquetas/   # revisar caso por caso
```

**Nunca** entra al repositorio: `.env`, credenciales, ni respaldos de la base.

Aparte de git, para producción hacen falta decisiones que no están tomadas y que **no son técnicas
sino de negocio** — quién autoriza qué, con qué respaldo y con qué ventana de indisponibilidad.
Cuatro que van a aparecer enseguida:

- **Secretos fuera de los archivos.** Hoy `JWT_SECRET` y la contraseña de Postgres están escritas en
  `docker-compose.yml`. En producción van en un gestor de secretos o en variables del ambiente.
- **HTTPS.** Hoy todo el tráfico va en claro; en el ambiente actual (una red de oficina) se asumió a
  conciencia, pero no es aceptable fuera de ahí.
- **Respaldos automáticos con retención y una restauración probada.** Un respaldo que nunca se
  restauró no se sabe si sirve.
- **Vida del refresh token.** 30 días es cómodo y largo; conviene decidirlo explícitamente.

---

## 11. Reglas de negocio que no se adivinan leyendo el código

Cada una costó medir datos reales. Cambiarlas sin entender por qué están rompe cosas silenciosamente.

### Bancos

- **Anti-duplicado:** `natural_key` + `indice_ocurrencia`. El mismo banco manda el mismo movimiento
  dos veces legítimamente (dos cargos idénticos el mismo día), así que la unicidad no puede ser solo
  la clave natural: lleva el número de ocurrencia.
- **Los meses vienen en inglés.** El Banco Popular exporta `01 JUN 2026`. Ocho de los doce meses se
  escriben igual en español, por eso el problema apareció recién en agosto. Cuando un importador no
  entiende **ninguna** fecha tiene que gritar, no reportar «0 movimientos».
- **Cada importación guarda el .xlsx original** en `importacion.archivo`. Cualquier falla se
  reproduce sin pedirle nada al usuario.
- **Los dólares no se convierten solos.** Registrar el TC del mes es un paso aparte; hasta que se
  haga, `monto_crc` está en cero para los movimientos en USD. Un total que mezcla monedas miente.
- **Traslado ≠ signo.** Un par de traslado exige las dos patas emparejadas, con tolerancia del **1 %**
  del monto (porcentaje, no monto fijo, por el diferencial cambiario en USD).
- **EBITDA por naturaleza declarada, no por signo.** `concepto.naturaleza` (INGRESO / GASTO / NEUTRO)
  la declara el usuario. Antes, derivarlo del signo hacía que el ahorro, las reservas y los aportes
  entre empresas infláramos los gastos de un mes en ₡35,3 millones. El default es NEUTRO y el
  tablero avisa qué quedó afuera.
- **Cierre bloqueante:** no se cierra un mes con movimientos sin clasificar, ni con actas de
  conciliación sin firmar.

### Cuentas por Pagar

- **Validación por riesgo — la regla está invertida a propósito.** Ninguna factura espera la
  conformidad del área **salvo** que dispare un criterio: monto sobre ₡250.000, proveedor con 2
  facturas históricas o menos, o desvío mayor al 50 % del promedio de ese mismo proveedor (solo
  sobre facturas de más de ₡100.000). Medido: **993 de 4.488 facturas (22,1 %) cubren el 89 % del
  dinero**. Antes se validaba todo y el 82 % de la cola era de ₡100.000 o menos, o sea el 8 % del
  monto. El veredicto se calcula **al revisar** y se congela (regla 6.4).
- **Una fase de la bandeja ya no es una lista de estados.** «Por aprobar» junta lo que el área validó
  con lo que nunca necesitó pasar por ella. De ahí `faseBandejaSQL` (regla 6.5).
- **Facturas «de Contabilidad»:** el gasto que ningún área puede validar (honorarios contables,
  timbres, comisiones bancarias, Hacienda). Se marca por factura, proveedor, concepto o
  clasificación, y hay **dos** permisos separados —marcar y aprobar— para que no sea autofirma.
- **Segregación de funciones:** quien validó el área no aprueba la misma factura; quien marcó a mano
  una factura como «de Contabilidad» tampoco puede firmarla.
- **La matriz de firmas se evalúa sobre el NETO**, no sobre el total: total menos anticipos aplicados.
- **Huella Bancos↔CxP:** la macro que va al banco lleva `CXP-xxxx`; al importar los pagos se
  emparejan con su factura **verificando el monto**. Monto distinto es un hallazgo, no una
  conciliación. Retroactivo no hay nada: cuando se midió, ni uno de los movimientos ya importados
  traía huella, y los `FE <n>` escritos a mano no calzan con ningún consecutivo.

### Cuentas por Cobrar

- **El sistema de origen no tiene documentos de cargo**, y de ahí salen todas las decisiones del
  módulo: cargos por período como partidas abiertas, sin factura electrónica, aplicación FIFO del
  más viejo, y el contrato como eje.
- **Suspensión por mora: 18 meses**, o su equivalencia según la modalidad (quincenal = 36 cuotas).
- **Los arreglos de pago no reescriben los cargos.** La cartera morosa se deriva.
- **La cola de cobro ordena por valor esperado sobre lo VENCIDO**, con la probabilidad del tramo y el
  factor de la forma de pago. Cambiar esos parámetros **reordena la cola**: la pantalla lo advierte.

### RRHH / Nómina

- **Guardarraíl legal, obligatorio.** Las comisiones y bonificaciones habituales **son salario** en
  Costa Rica. Está **prohibido** construir funciones o reportes cuyo propósito sea cuantificar u
  optimizar el «ahorro por no reportar» a la CCSS. Sí se permiten conceptos legítimamente no
  salariales (viáticos, reembolsos) con su base legal declarada.
- **Parámetros verificados 2026:** obrero 10,83 %, patronal 26,83 %. Versionados por año.
- **Dos tratamientos según la jornada:** mensual y quincenal (adelanto configurable + liquidación).
- **Horas extra: se capturan HORAS**, y el monto se deriva (horas × valor hora × factor). El factor
  nunca baja de 1,5 — el artículo 139 es un piso, no un parámetro. El monto **no se guarda**: se
  recalcula con el salario vigente.
- **El saldo de vacaciones se deriva**, no se almacena.
- **El SINPE y la planilla CCSS del mes incluyen los finiquitos** (huella `FIN-`), una fila por
  trabajador.

### Reportes en Excel

- El formato vive en `ConstruirLibro`. Tres trampas que ya costaron: estilar un rango **combinado**
  solo en la primera celda deja líneas sueltas en la columna A; una columna de monto angosta muestra
  `2,5E+07`; y `time.Now()` sella en UTC dentro del contenedor (usar `AhoraCR()`).
- Exportar por HTTP necesita **`Content-Disposition` expuesto en CORS**, o el navegador descarta el
  nombre del archivo y la descarga sale con nombre genérico.

---

## 12. Limitaciones conocidas

Se entregan dichas, no escondidas:

| Limitación | Impacto | Dónde |
|---|---|---|
| **La auditoría no guarda el valor anterior** | Sirve para saber quién hizo qué, **no alcanza para revertir**. Ante un error masivo, la única vuelta atrás es el respaldo | `auditoria_evento` |
| **Una importación bancaria no se puede deshacer** | No hay endpoint para excluir o revertir una importación. El cambio más chico sería marcar `incluido = false` por importación: los ~20 queries que ya filtran por `incluido` lo respetarían solos | `bancos` |
| **Cerrar un período es irreversible** | No existe endpoint para reabrir | `periodo_cierre` |
| **Sin límite de intentos de login** | No hay bloqueo por fuerza bruta. Cada ingreso queda auditado, pero no se frena | `auth` |
| **La contraseña temporal solo bloquea el navegador** | Quien llame la API directo puede operar sin cambiarla. Se cierra no emitiendo el refresh token cuando `debe_cambiar_password` es true | `auth` |
| **El frontend corre en modo desarrollo** | Vite dev server, no build estático servido por nginx. Funciona, pero para producción hay que cambiarlo | `frontend/Dockerfile` |
| **Sin pruebas de integración con base real** | Las pruebas de servicio usan repositorios falsos. No hay `testcontainers` con PostgreSQL de verdad | `*_test.go` |
| **18 descripciones bancarias con caracteres corruptos** | `DueÃ±o` en vez de `Dueño`. Sin corregir a propósito, pendiente de decisión | `movimiento_bancario` |
| **30 pares de traslados en AMBIGUO** | Esperan resolución manual | `movimiento_bancario` |

---

## 13. Sobre «documentar cada función»

Una aclaración honesta sobre el alcance de este manual.

Documentar función por función 40.805 líneas de Go daría un texto enorme que quedaría desactualizado
en la primera semana, y que nadie leería. Lo que sí sirve, y es lo que se entrega:

1. **La superficie completa de la API** — `ENDPOINTS.md`, con las 247 rutas y el permiso de cada una,
   **generado del router** para que no pueda mentir.
2. **El esquema completo** — `schema.sql`, extraído de la base real y probado.
3. **La arquitectura** (§7): sabiendo que todo es handler → service → repository, encontrar cualquier
   función es directo. `router.go` funciona como índice: de la ruta se llega al handler, del handler
   al service, del service al repository.
4. **Las reglas que no se deducen del código** (§6 y §11). Esto es lo que de verdad no se puede
   reconstruir leyendo, y es donde se rompen las cosas.

El código está comentado en español y los comentarios explican **por qué**, no qué — incluyendo, en
varios casos, el error concreto que motivó la decisión. Esa es la documentación función por función,
y vive junto al código, que es donde no se desactualiza.

Si el equipo quiere una referencia formal de la API para generar clientes, el camino es publicar
**OpenAPI** desde el router. Hay un `docs/openapi-cxp.yaml` parcial, mantenido a mano, que hoy solo
cubre CxP.
