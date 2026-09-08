# GPVDP ERP — Diccionario de datos

**Generado desde el catálogo de PostgreSQL de la base en operación.** No escrito a mano: refleja
exactamente lo que existe hoy. Se regenera cuando cambia el esquema.

**86 tablas · 936 columnas**

## Cómo leer las tablas

- **PK** — forma parte de la clave primaria.
- **→** — clave foránea, con la tabla y columna a la que apunta.
- **NN** — `NOT NULL`: la columna es obligatoria.
- **Default** — valor que pone PostgreSQL si no se indica uno.

## Convenciones de tipo (respetarlas al agregar columnas)

| Para | Tipo a usar |
|---|---|
| Identificador | `uuid DEFAULT gen_random_uuid()` |
| Texto | `text` — **nunca** `varchar(n)` |
| Dinero | `numeric(14,2)` (o 16,2 / 18,2 si el monto lo pide). **Jamás** `float`, `double precision` ni `real` |
| Fecha con hora | `timestamptz` — **siempre** con zona |
| Fecha sola | `date` |
| Bandera | `boolean` |
| Tipo de cambio | `numeric(14,4)` |
| Porcentaje | `numeric(5,2)` |
| Estructura variable | `jsonb` |

**`empresa_id uuid NOT NULL`** va en toda tabla con datos de una empresa, y **toda** consulta filtra
por ella tomándola del token. Ver el manual técnico, sección 6.1.

---

## Núcleo, seguridad y auditoría

10 tablas, 60 columnas.

### `empresa`

6 columnas.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `id` | `uuid` | **NN** | `gen_random_uuid()` | **PK** |
| `nombre` | `text` | **NN** | — | — |
| `tipo_legal` | `text` | sí | — | — |
| `activo` | `boolean` | **NN** | `true` | — |
| `creado_en` | `timestamptz` | **NN** | `now()` | — |
| `tolerancia_traslado` | `numeric(6,4)` | **NN** | `0.01` | — |

### `usuario`

8 columnas.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `id` | `uuid` | **NN** | `gen_random_uuid()` | **PK** |
| `nombre` | `text` | **NN** | — | — |
| `email` | `text` | **NN** | — | — |
| `password_hash` | `text` | **NN** | — | — |
| `activo` | `boolean` | **NN** | `true` | — |
| `creado_en` | `timestamptz` | **NN** | `now()` | — |
| `actualizado_en` | `timestamptz` | **NN** | `now()` | — |
| `debe_cambiar_password` | `boolean` | **NN** | `false` | — |

### `usuario_empresa_rol`

5 columnas.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `id` | `uuid` | **NN** | `gen_random_uuid()` | **PK** |
| `empresa_id` | `uuid` | **NN** | — | → `empresa.id` |
| `usuario_id` | `uuid` | **NN** | — | → `usuario.id` |
| `rol_id` | `uuid` | **NN** | — | → `rol.id` |
| `creado_en` | `timestamptz` | **NN** | `now()` | — |

### `rol`

6 columnas.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `id` | `uuid` | **NN** | `gen_random_uuid()` | **PK** |
| `codigo` | `text` | **NN** | — | — |
| `nombre` | `text` | **NN** | — | — |
| `descripcion` | `text` | sí | — | — |
| `creado_en` | `timestamptz` | **NN** | `now()` | — |
| `empresa_id` | `uuid` | sí | — | → `empresa.id` |

### `permiso`

7 columnas.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `id` | `uuid` | **NN** | `gen_random_uuid()` | **PK** |
| `codigo` | `text` | **NN** | — | — |
| `modulo` | `text` | **NN** | — | — |
| `nombre` | `text` | **NN** | — | — |
| `descripcion` | `text` | sí | — | — |
| `critico` | `boolean` | **NN** | `false` | — |
| `orden` | `integer` | **NN** | `0` | — |

### `rol_permiso`

4 columnas.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `empresa_id` | `uuid` | **NN** | — | **PK** → `empresa.id` |
| `rol_id` | `uuid` | **NN** | — | **PK** → `rol.id` |
| `permiso_id` | `uuid` | **NN** | — | **PK** → `permiso.id` |
| `creado_en` | `timestamptz` | **NN** | `now()` | — |

### `rol_clasificacion_consulta`

El ALCANCE de la consulta por segmento (mig 0077): qué partidas de gasto/ingreso puede consultar
cada rol en la pantalla «Mi partida». Es una capa distinta del permiso: `bancos.ver_mi_segmento`
dice **qué pantalla** abre el rol; esta tabla dice **cuáles filas** ve ahí.

Sin filas para un rol, ese rol no ve NADA (nunca «todo»): el servicio corta antes de consultar y
el repositorio agrega una condición imposible. Es el estado normal de casi todos los roles.

4 columnas.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `empresa_id` | `uuid` | **NN** | — | **PK** → `empresa.id` |
| `rol_id` | `uuid` | **NN** | — | **PK** → `rol.id` |
| `clasificacion_id` | `uuid` | **NN** | — | **PK** → `clasificacion.id` |
| `creado_en` | `timestamptz` | **NN** | `now()` | — |

### `movimiento_reporte_segmentacion`

Los avisos «este movimiento está mal segmentado» que levantan los equipos de consulta. El equipo
señala; corregir la partida sigue siendo de quien clasifica.

El estado se DERIVA de `resuelto_en` (nulo = pendiente): no hay columna «estado» que mantener
sincronizada. Un índice único parcial sobre `movimiento_id WHERE resuelto_en IS NULL` deja **un
solo aviso abierto por movimiento**, para que tres personas avisando del mismo no generen tres
colas. El histórico de avisos resueltos del mismo movimiento sí admite varias filas.

13 columnas.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `id` | `uuid` | **NN** | `gen_random_uuid()` | **PK** |
| `empresa_id` | `uuid` | **NN** | — | → `empresa.id` |
| `movimiento_id` | `uuid` | sí | — | → `movimiento_bancario.id`. **Nulo en un aviso de FALTANTE** que no se pudo enganchar (mig 0078) |
| `usuario_id` | `uuid` | **NN** | — | → `usuario.id` (quién avisó) |
| `motivo` | `text` | **NN** | — | obligatorio: sin él no se puede corregir |
| `creado_en` | `timestamptz` | **NN** | `now()` | — |
| `resuelto_en` | `timestamptz` | sí | — | nulo = pendiente |
| `resuelto_por` | `uuid` | sí | — | → `usuario.id` |
| `resolucion` | `text` | sí | — | `RECLASIFICADO` \| `SIN_CAMBIO` |
| `respuesta` | `text` | sí | — | obligatoria si `SIN_CAMBIO`; la ve el equipo |
| `fecha_esperada` | `date` | sí | — | Aviso de FALTANTE: el día en que el equipo esperaba el movimiento |
| `monto_esperado` | `numeric(16,2)` | sí | — | Aviso de FALTANTE: el monto exacto que esperaba |
| `referencia` | `text` | sí | — | Nº de recibo o comprobante con el que buscarlo |

### `sesion`

6 columnas.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `id` | `uuid` | **NN** | `gen_random_uuid()` | **PK** |
| `usuario_id` | `uuid` | **NN** | — | → `usuario.id` |
| `token_hash` | `text` | **NN** | — | — |
| `expira_en` | `timestamptz` | **NN** | — | — |
| `revocado` | `boolean` | **NN** | `false` | — |
| `creado_en` | `timestamptz` | **NN** | `now()` | — |

### `auditoria_evento`

9 columnas.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `id` | `uuid` | **NN** | `gen_random_uuid()` | **PK** |
| `empresa_id` | `uuid` | sí | — | → `empresa.id` |
| `entidad` | `text` | **NN** | — | — |
| `entidad_id` | `uuid` | sí | — | — |
| `accion` | `text` | **NN** | — | — |
| `valor_anterior` | `jsonb` | sí | — | — |
| `valor_nuevo` | `jsonb` | sí | — | — |
| `usuario_id` | `uuid` | sí | — | → `usuario.id` |
| `ts` | `timestamptz` | **NN** | `now()` | — |

### `plantilla_correo`

7 columnas.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `id` | `uuid` | **NN** | `gen_random_uuid()` | **PK** |
| `empresa_id` | `uuid` | **NN** | — | → `empresa.id` |
| `clave` | `text` | **NN** | — | — |
| `asunto` | `text` | **NN** | — | — |
| `cuerpo` | `text` | **NN** | — | — |
| `actualizado_por` | `uuid` | sí | — | → `usuario.id` |
| `actualizado_en` | `timestamptz` | **NN** | `now()` | — |

### `schema_migrations`

2 columnas.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `version` | `bigint` | **NN** | — | **PK** |
| `dirty` | `boolean` | **NN** | — | — |

## Bancos

18 tablas, 172 columnas.

### `banco`

5 columnas.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `id` | `uuid` | **NN** | `gen_random_uuid()` | **PK** |
| `empresa_id` | `uuid` | **NN** | — | → `empresa.id` |
| `nombre` | `text` | **NN** | — | — |
| `activo` | `boolean` | **NN** | `true` | — |
| `creado_en` | `timestamptz` | **NN** | `now()` | — |

### `cuenta_bancaria`

8 columnas.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `id` | `uuid` | **NN** | `gen_random_uuid()` | **PK** |
| `empresa_id` | `uuid` | **NN** | — | → `empresa.id` |
| `banco_id` | `uuid` | **NN** | — | → `banco.id` |
| `iban` | `text` | sí | — | — |
| `moneda` | `text` | **NN** | — | — |
| `alias` | `text` | sí | — | — |
| `activo` | `boolean` | **NN** | `true` | — |
| `creado_en` | `timestamptz` | **NN** | `now()` | — |

### `concepto`

9 columnas.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `id` | `uuid` | **NN** | `gen_random_uuid()` | **PK** |
| `empresa_id` | `uuid` | **NN** | — | → `empresa.id` |
| `nombre` | `text` | **NN** | — | — |
| `activo` | `boolean` | **NN** | `true` | — |
| `creado_en` | `timestamptz` | **NN** | `now()` | — |
| `visible_cxp` | `boolean` | **NN** | `true` | — |
| `es_contabilidad` | `boolean` | **NN** | `false` | — |
| `naturaleza` | `text` | **NN** | `'NEUTRO'::text` | — |
| `naturaleza_declarada` | `boolean` | **NN** | `false` | Separa la decisión del silencio: false = nadie la declaró y el valor de `naturaleza` viene del default |

### `clasificacion`

8 columnas.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `id` | `uuid` | **NN** | `gen_random_uuid()` | **PK** |
| `empresa_id` | `uuid` | **NN** | — | → `empresa.id` |
| `concepto_id` | `uuid` | **NN** | — | → `concepto.id` |
| `nombre` | `text` | **NN** | — | — |
| `cuenta_contable_futura` | `text` | sí | — | — |
| `activo` | `boolean` | **NN** | `true` | — |
| `creado_en` | `timestamptz` | **NN** | `now()` | — |
| `es_contabilidad` | `boolean` | **NN** | `false` | — |

### `subclasificacion`

6 columnas.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `id` | `uuid` | **NN** | `gen_random_uuid()` | **PK** |
| `empresa_id` | `uuid` | **NN** | — | → `empresa.id` |
| `clasificacion_id` | `uuid` | **NN** | — | → `clasificacion.id` |
| `nombre` | `text` | **NN** | — | — |
| `activo` | `boolean` | **NN** | `true` | — |
| `creado_en` | `timestamptz` | **NN** | `now()` | — |

### `movimiento_bancario`

26 columnas.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `id` | `uuid` | **NN** | `gen_random_uuid()` | **PK** |
| `empresa_id` | `uuid` | **NN** | — | → `empresa.id` |
| `cuenta_bancaria_id` | `uuid` | **NN** | — | → `cuenta_bancaria.id` |
| `importacion_id` | `uuid` | sí | — | → `importacion.id` |
| `fecha` | `date` | **NN** | — | — |
| `documento` | `text` | sí | — | — |
| `descripcion` | `text` | sí | — | — |
| `debito` | `numeric(16,2)` | **NN** | `0` | — |
| `credito` | `numeric(16,2)` | **NN** | `0` | — |
| `moneda_original` | `text` | **NN** | `'CRC'::text` | — |
| `monto_original` | `numeric(16,2)` | **NN** | `0` | — |
| `monto_crc` | `numeric(16,2)` | **NN** | `0` | — |
| `tc_aplicado` | `numeric(14,4)` | sí | — | — |
| `concepto_id` | `uuid` | sí | — | → `clasificacion.id` → `clasificacion.concepto_id` → `concepto.id` |
| `clasificacion_id` | `uuid` | sí | — | → `clasificacion.id` → `clasificacion.concepto_id` |
| `estado_clasificacion` | `text` | **NN** | `'NO_IDENTIFICADO'::text` | — |
| `confianza` | `numeric(5,2)` | sí | — | — |
| `es_traslado` | `boolean` | **NN** | `false` | — |
| `par_traslado_id` | `uuid` | sí | — | → `movimiento_bancario.id` |
| `natural_key` | `text` | **NN** | — | — |
| `indice_ocurrencia` | `integer` | **NN** | `1` | — |
| `incluido` | `boolean` | **NN** | `true` | — |
| `origen_historico` | `boolean` | **NN** | `false` | — |
| `creado_en` | `timestamptz` | **NN** | `now()` | — |
| `actualizado_en` | `timestamptz` | **NN** | `now()` | — |
| `documento_cxp_id` | `uuid` | sí | — | → `documento_cxp.id` |

### `importacion`

10 columnas.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `id` | `uuid` | **NN** | `gen_random_uuid()` | **PK** |
| `empresa_id` | `uuid` | **NN** | — | → `empresa.id` |
| `cuenta_bancaria_id` | `uuid` | **NN** | — | → `cuenta_bancaria.id` |
| `source_file_hash` | `text` | **NN** | — | — |
| `nombre_archivo` | `text` | **NN** | — | — |
| `estado` | `text` | **NN** | `'CARGADA'::text` | — |
| `creado_por` | `uuid` | sí | — | → `usuario.id` |
| `creado_en` | `timestamptz` | **NN** | `now()` | — |
| `archivo` | `bytea` | sí | — | — |
| `banco` | `text` | sí | — | — |

### `regla_clasificacion`

10 columnas.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `id` | `uuid` | **NN** | `gen_random_uuid()` | **PK** |
| `empresa_id` | `uuid` | **NN** | — | → `empresa.id` |
| `nombre` | `text` | **NN** | — | — |
| `aplica_a` | `text` | **NN** | — | — |
| `concepto_id` | `uuid` | **NN** | — | → `concepto.id` → `clasificacion.id` → `clasificacion.concepto_id` |
| `clasificacion_id` | `uuid` | **NN** | — | → `clasificacion.id` → `clasificacion.concepto_id` |
| `prioridad` | `integer` | **NN** | `100` | — |
| `activo` | `boolean` | **NN** | `true` | — |
| `creado_en` | `timestamptz` | **NN** | `now()` | — |
| `aciertos` | `integer` | **NN** | `0` | — |

### `palabra_clave`

4 columnas.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `id` | `uuid` | **NN** | `gen_random_uuid()` | **PK** |
| `regla_id` | `uuid` | **NN** | — | → `regla_clasificacion.id` |
| `texto` | `text` | **NN** | — | — |
| `creado_en` | `timestamptz` | **NN** | `now()` | — |

### `proveedor_gasto`

8 columnas.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `id` | `uuid` | **NN** | `gen_random_uuid()` | **PK** |
| `empresa_id` | `uuid` | **NN** | — | → `empresa.id` |
| `proveedor_id` | `uuid` | **NN** | — | → `proveedor.id` |
| `concepto_id` | `uuid` | **NN** | — | → `concepto.id` |
| `clasificacion_id` | `uuid` | sí | — | → `clasificacion.id` |
| `subclasificacion_id` | `uuid` | sí | — | → `subclasificacion.id` |
| `usos` | `integer` | **NN** | `1` | — |
| `ultimo_uso` | `timestamptz` | **NN** | `now()` | — |

### `tipo_cambio_mes`

8 columnas.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `id` | `uuid` | **NN** | `gen_random_uuid()` | **PK** |
| `empresa_id` | `uuid` | **NN** | — | → `empresa.id` |
| `anio` | `integer` | **NN** | — | — |
| `mes` | `integer` | **NN** | — | — |
| `valor_congelado` | `numeric(14,4)` | sí | — | — |
| `estado` | `text` | **NN** | `'PROVISIONAL'::text` | — |
| `congelado_en` | `timestamptz` | sí | — | — |
| `creado_en` | `timestamptz` | **NN** | `now()` | — |

### `tipo_cambio_cotizacion`

6 columnas.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `id` | `uuid` | **NN** | `gen_random_uuid()` | **PK** |
| `empresa_id` | `uuid` | **NN** | — | → `empresa.id` |
| `fecha` | `date` | **NN** | — | — |
| `valor` | `numeric(14,4)` | **NN** | — | — |
| `fuente` | `text` | **NN** | — | — |
| `creado_en` | `timestamptz` | **NN** | `now()` | — |

### `bccr_sync_log`

8 columnas.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `id` | `uuid` | **NN** | `gen_random_uuid()` | **PK** |
| `empresa_id` | `uuid` | **NN** | — | → `empresa.id` |
| `fecha` | `date` | **NN** | — | — |
| `indicador` | `text` | **NN** | — | — |
| `valor` | `numeric(14,4)` | sí | — | — |
| `exito` | `boolean` | **NN** | — | — |
| `mensaje` | `text` | sí | — | — |
| `creado_en` | `timestamptz` | **NN** | `now()` | — |

### `periodo_cierre`

7 columnas.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `id` | `uuid` | **NN** | `gen_random_uuid()` | **PK** |
| `empresa_id` | `uuid` | **NN** | — | → `empresa.id` |
| `anio` | `integer` | **NN** | — | — |
| `mes` | `integer` | **NN** | — | — |
| `no_identificados_al_cierre` | `integer` | **NN** | `0` | — |
| `cerrado_por` | `uuid` | sí | — | → `usuario.id` |
| `cerrado_en` | `timestamptz` | **NN** | `now()` | — |

### `proyeccion_escenario`

13 columnas.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `id` | `uuid` | **NN** | `gen_random_uuid()` | **PK** |
| `empresa_id` | `uuid` | **NN** | — | → `empresa.id` |
| `periodo` | `text` | **NN** | — | — |
| `metodo` | `text` | **NN** | — | — |
| `metodo_efectivo` | `text` | **NN** | — | — |
| `meta_crecimiento_pct` | `numeric(6,2)` | **NN** | `0` | — |
| `lineas_ingreso` | `text[]` | **NN** | `'{}'::text[]` | — |
| `dia_calculo` | `integer` | **NN** | — | — |
| `real_acumulado` | `numeric(18,2)` | **NN** | — | — |
| `cierre_proyectado` | `numeric(18,2)` | **NN** | — | — |
| `meta_monto` | `numeric(18,2)` | **NN** | — | — |
| `creado_por` | `uuid` | sí | — | — |
| `creado_en` | `timestamptz` | **NN** | `now()` | — |

### `saldo_cuenta_diario`

11 columnas.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `id` | `uuid` | **NN** | `gen_random_uuid()` | **PK** |
| `empresa_id` | `uuid` | **NN** | — | → `empresa.id` |
| `cuenta_bancaria_id` | `uuid` | **NN** | — | → `cuenta_bancaria.id` |
| `fecha` | `date` | **NN** | — | — |
| `saldo` | `numeric(16,2)` | **NN** | — | — |
| `nota` | `text` | sí | — | — |
| `capturado_por` | `uuid` | sí | — | → `usuario.id` |
| `capturado_en` | `timestamptz` | **NN** | `now()` | — |
| `actualizado_en` | `timestamptz` | **NN** | `now()` | — |
| `revisado_por` | `uuid` | sí | — | → `usuario.id` |
| `revisado_en` | `timestamptz` | sí | — | — |

### `acta_conciliacion`

12 columnas.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `id` | `uuid` | **NN** | `gen_random_uuid()` | **PK** |
| `empresa_id` | `uuid` | **NN** | — | → `empresa.id` |
| `cuenta_bancaria_id` | `uuid` | **NN** | — | → `cuenta_bancaria.id` |
| `anio` | `integer` | **NN** | — | — |
| `mes` | `integer` | **NN** | — | — |
| `saldo_banco` | `numeric(16,2)` | **NN** | — | — |
| `saldo_libros` | `numeric(16,2)` | **NN** | — | — |
| `ajuste_partidas` | `numeric(16,2)` | **NN** | — | — |
| `preparado_por` | `uuid` | sí | — | → `usuario.id` |
| `preparado_en` | `timestamptz` | **NN** | `now()` | — |
| `firmado_por` | `uuid` | sí | — | → `usuario.id` |
| `firmado_en` | `timestamptz` | sí | — | — |

### `partida_conciliacion`

14 columnas.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `id` | `uuid` | **NN** | `gen_random_uuid()` | **PK** |
| `empresa_id` | `uuid` | **NN** | — | → `empresa.id` |
| `cuenta_bancaria_id` | `uuid` | **NN** | — | → `cuenta_bancaria.id` |
| `anio` | `integer` | **NN** | — | — |
| `mes` | `integer` | **NN** | — | — |
| `tipo` | `text` | **NN** | — | — |
| `descripcion` | `text` | **NN** | — | — |
| `monto` | `numeric(16,2)` | **NN** | — | — |
| `signo` | `smallint` | **NN** | — | — |
| `anulada` | `boolean` | **NN** | `false` | — |
| `registrado_por` | `uuid` | sí | — | → `usuario.id` |
| `registrado_en` | `timestamptz` | **NN** | `now()` | — |
| `anulada_por` | `uuid` | sí | — | → `usuario.id` |
| `anulada_en` | `timestamptz` | sí | — | — |

## Cuentas por Pagar (CxP)

11 tablas, 136 columnas.

### `proveedor`

20 columnas.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `id` | `uuid` | **NN** | `gen_random_uuid()` | **PK** |
| `empresa_id` | `uuid` | **NN** | — | → `empresa.id` |
| `nombre` | `text` | **NN** | — | — |
| `tipo_identificacion` | `text` | sí | — | — |
| `identificacion` | `text` | sí | — | — |
| `email` | `text` | sí | — | — |
| `telefono` | `text` | sí | — | — |
| `iban` | `text` | sí | — | — |
| `retencion_renta_pct` | `numeric(5,2)` | **NN** | `0` | — |
| `exento_iva` | `boolean` | **NN** | `false` | — |
| `activo` | `boolean` | **NN** | `true` | — |
| `creado_en` | `timestamptz` | **NN** | `now()` | — |
| `actualizado_en` | `timestamptz` | **NN** | `now()` | — |
| `gasto_concepto_id` | `uuid` | sí | — | → `concepto.id` |
| `gasto_clasificacion_id` | `uuid` | sí | — | → `clasificacion.id` |
| `gasto_subclasificacion_id` | `uuid` | sí | — | → `subclasificacion.id` |
| `condicion_pago` | `text` | **NN** | `'CONTADO'::text` | — |
| `plazo_credito_dias` | `integer` | **NN** | `0` | — |
| `departamento` | `text` | sí | — | — |
| `es_contabilidad` | `boolean` | **NN** | `false` | — |

### `documento_cxp`

43 columnas.

**`bloqueado_para_pago` es un candado contra el doble pago, y es una marca POSITIVA a propósito.** Las
tres consultas que arman el archivo de pagos filtran por `estado = 'PROGRAMADO'`; ninguna mira el
tipo. La provisión que genera Inventario al usar un cofre consignado es de tipo `INTERNO` —vía
expresa, aprobable directo desde RECIBIDO—, así que recorría ese camino completo: si alguien la pagaba
antes de conciliarla, después entraba la factura electrónica real del proveedor y se pagaba el mismo
cofre dos veces. Con la marca, `Programar` la rechaza (y el mensaje dice el motivo) y el archivo de
pagos la excluye. El default en `false` deja intactas las facturas existentes: solo afecta lo que se
marque a propósito. Negar tipo por tipo sería peor — bastaría que una consulta futura se olvidara de
la lista.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `id` | `uuid` | **NN** | `gen_random_uuid()` | **PK** |
| `empresa_id` | `uuid` | **NN** | — | → `empresa.id` |
| `proveedor_id` | `uuid` | **NN** | — | → `proveedor.id` |
| `clave` | `text` | **NN** | — | — |
| `consecutivo` | `text` | sí | — | — |
| `fecha_emision` | `date` | **NN** | — | — |
| `moneda` | `text` | **NN** | `'CRC'::text` | — |
| `subtotal` | `numeric(16,2)` | **NN** | `0` | — |
| `iva` | `numeric(16,2)` | **NN** | `0` | — |
| `retencion` | `numeric(16,2)` | **NN** | `0` | — |
| `total` | `numeric(16,2)` | **NN** | `0` | — |
| `tc_aplicado` | `numeric(14,4)` | sí | — | — |
| `total_crc` | `numeric(16,2)` | **NN** | `0` | — |
| `descripcion` | `text` | sí | — | — |
| `estado` | `text` | **NN** | `'RECIBIDO'::text` | — |
| `fecha_pago_programada` | `date` | sí | — | — |
| `huella` | `text` | sí | — | — |
| `creado_por` | `uuid` | sí | — | → `usuario.id` |
| `creado_en` | `timestamptz` | **NN** | `now()` | — |
| `actualizado_en` | `timestamptz` | **NN** | `now()` | — |
| `concepto_id` | `uuid` | sí | — | → `concepto.id` |
| `clasificacion_id` | `uuid` | sí | — | → `clasificacion.id` |
| `fecha_vencimiento` | `date` | sí | — | — |
| `tipo` | `text` | **NN** | `'CXP'::text` | — |
| `subclasificacion_id` | `uuid` | sí | — | → `subclasificacion.id` |
| `lote_id` | `uuid` | sí | — | → `lote_pago.id` |
| `comprobante_enviado_en` | `timestamptz` | sí | — | — |
| `clasif_auto` | `boolean` | **NN** | `false` | — |
| `prioridad` | `text` | **NN** | `''::text` | — |
| `nota_revision` | `text` | sí | — | — |
| `departamento_id` | `uuid` | sí | — | → `departamento.id` |
| `validado_depto_por` | `uuid` | sí | — | — |
| `validado_depto_en` | `timestamptz` | sí | — | — |
| `validacion_respaldo` | `text` | sí | — | — |
| `validacion_nota` | `text` | sí | — | — |
| `es_contabilidad` | `boolean` | sí | — | — |
| `contabilidad_motivo` | `text` | sí | — | — |
| `contabilidad_marcado_por` | `uuid` | sí | — | → `usuario.id` |
| `contabilidad_marcado_en` | `timestamptz` | sí | — | — |
| `requiere_validacion` | `boolean` | sí | — | — |
| `validacion_motivo` | `text` | sí | — | — |
| `bloqueado_para_pago` | `boolean` | **NN** | `false` | no se puede programar ni entrar al archivo del banco (mig 0072) |
| `bloqueo_motivo` | `text` | sí | — | por qué está bloqueado, en palabras |

### `documento_cxp_aprobacion`

6 columnas.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `id` | `uuid` | **NN** | `gen_random_uuid()` | **PK** |
| `empresa_id` | `uuid` | **NN** | — | → `empresa.id` |
| `documento_id` | `uuid` | **NN** | — | → `documento_cxp.id` |
| `usuario_id` | `uuid` | **NN** | — | → `usuario.id` |
| `rol` | `text` | **NN** | — | — |
| `aprobado_en` | `timestamptz` | **NN** | `now()` | — |

### `anticipo_aplicacion`

10 columnas.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `id` | `uuid` | **NN** | `gen_random_uuid()` | **PK** |
| `empresa_id` | `uuid` | **NN** | — | → `empresa.id` |
| `anticipo_id` | `uuid` | **NN** | — | → `documento_cxp.id` |
| `factura_id` | `uuid` | **NN** | — | → `documento_cxp.id` |
| `monto_crc` | `numeric(14,2)` | **NN** | — | — |
| `aplicado_por` | `uuid` | sí | — | → `usuario.id` |
| `aplicado_en` | `timestamptz` | **NN** | `now()` | — |
| `activo` | `boolean` | **NN** | `true` | — |
| `reversado_por` | `uuid` | sí | — | → `usuario.id` |
| `reversado_en` | `timestamptz` | sí | — | — |

### `departamento`

9 columnas.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `id` | `uuid` | **NN** | `gen_random_uuid()` | **PK** |
| `empresa_id` | `uuid` | **NN** | — | → `empresa.id` |
| `nombre` | `text` | **NN** | — | — |
| `codigo` | `text` | sí | — | — |
| `centro_costo` | `text` | sí | — | — |
| `activo` | `boolean` | **NN** | `true` | — |
| `orden` | `integer` | **NN** | `0` | — |
| `creado_en` | `timestamptz` | **NN** | `now()` | — |
| `actualizado_en` | `timestamptz` | **NN** | `now()` | — |

### `departamento_validador`

4 columnas.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `departamento_id` | `uuid` | **NN** | — | **PK** → `departamento.id` |
| `usuario_id` | `uuid` | **NN** | — | **PK** → `usuario.id` |
| `rol` | `text` | **NN** | `'TITULAR'::text` | — |
| `creado_en` | `timestamptz` | **NN** | `now()` | — |

### `lote_pago`

7 columnas.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `id` | `uuid` | **NN** | `gen_random_uuid()` | **PK** |
| `empresa_id` | `uuid` | **NN** | — | → `empresa.id` |
| `numero` | `bigint` | **NN** | — | — |
| `fecha_corte` | `date` | **NN** | — | — |
| `estado` | `text` | **NN** | `'ABIERTO'::text` | — |
| `creado_por` | `uuid` | sí | — | → `usuario.id` |
| `creado_en` | `timestamptz` | **NN** | `now()` | — |

### `comprobante_pago`

8 columnas.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `id` | `uuid` | **NN** | `gen_random_uuid()` | **PK** |
| `empresa_id` | `uuid` | **NN** | — | → `empresa.id` |
| `documento_id` | `uuid` | **NN** | — | → `documento_cxp.id` |
| `filename` | `text` | **NN** | — | — |
| `mime` | `text` | **NN** | `'application/pdf'::text` | — |
| `contenido` | `bytea` | **NN** | — | — |
| `subido_por` | `uuid` | sí | — | → `usuario.id` |
| `subido_en` | `timestamptz` | **NN** | `now()` | — |

### `caja_chica_fondo`

11 columnas.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `id` | `uuid` | **NN** | `gen_random_uuid()` | **PK** |
| `empresa_id` | `uuid` | **NN** | — | → `empresa.id` |
| `nombre` | `text` | **NN** | — | — |
| `custodio_id` | `uuid` | sí | — | → `usuario.id` |
| `departamento_id` | `uuid` | sí | — | → `departamento.id` |
| `proveedor_id` | `uuid` | sí | — | → `proveedor.id` |
| `monto_asignado` | `numeric(14,2)` | **NN** | — | — |
| `umbral_pct` | `numeric(5,2)` | **NN** | `30` | — |
| `limite_vale` | `numeric(14,2)` | **NN** | `0` | — |
| `activo` | `boolean` | **NN** | `true` | — |
| `creado_en` | `timestamptz` | **NN** | `now()` | — |

### `caja_chica_vale`

14 columnas.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `id` | `uuid` | **NN** | `gen_random_uuid()` | **PK** |
| `empresa_id` | `uuid` | **NN** | — | → `empresa.id` |
| `fondo_id` | `uuid` | **NN** | — | → `caja_chica_fondo.id` |
| `fecha` | `date` | **NN** | `CURRENT_DATE` | — |
| `detalle` | `text` | **NN** | — | — |
| `monto_crc` | `numeric(14,2)` | **NN** | — | — |
| `concepto_id` | `uuid` | sí | — | → `concepto.id` |
| `clasificacion_id` | `uuid` | sí | — | → `clasificacion.id` |
| `subclasificacion_id` | `uuid` | sí | — | → `subclasificacion.id` |
| `comprobante` | `text` | **NN** | `'RECIBO'::text` | — |
| `registrado_por` | `uuid` | sí | — | → `usuario.id` |
| `reposicion_id` | `uuid` | sí | — | → `documento_cxp.id` |
| `anulado` | `boolean` | **NN** | `false` | — |
| `creado_en` | `timestamptz` | **NN** | `now()` | — |

### `cxp_parametro`

6 columnas.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `empresa_id` | `uuid` | **NN** | — | **PK** → `empresa.id` |
| `clave` | `text` | **NN** | — | **PK** |
| `valor` | `text` | **NN** | — | — |
| `descripcion` | `text` | **NN** | `''::text` | — |
| `actualizado_en` | `timestamptz` | **NN** | `now()` | — |
| `actualizado_por` | `uuid` | sí | — | → `usuario.id` |

## Cuentas por Cobrar (CxC)

23 tablas, 251 columnas.

### `contrato_cxc`

28 columnas.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `id` | `uuid` | **NN** | `gen_random_uuid()` | **PK** |
| `empresa_id` | `uuid` | **NN** | — | → `empresa.id` |
| `numero` | `text` | **NN** | — | — |
| `sede_id` | `uuid` | sí | — | → `cxc_sede.id` |
| `cliente_nombre` | `text` | **NN** | `''::text` | — |
| `documento` | `text` | **NN** | `''::text` | — |
| `telefonos` | `text` | **NN** | `''::text` | — |
| `correos` | `text` | **NN** | `''::text` | — |
| `servicio` | `text` | **NN** | `''::text` | — |
| `tipo_servicio` | `text` | **NN** | `''::text` | — |
| `modalidad_id` | `uuid` | sí | — | → `cxc_modalidad.id` |
| `forma_pago_id` | `uuid` | sí | — | → `cxc_forma_pago.id` |
| `asociacion_id` | `uuid` | sí | — | → `cxc_asociacion.id` |
| `dia_pago` | `smallint` | sí | — | — |
| `cuota_vigente` | `numeric(14,2)` | **NN** | `0` | — |
| `fecha_inicial` | `date` | sí | — | — |
| `fecha_primer_cobro` | `date` | sí | — | — |
| `tarjeta_vence` | `date` | sí | — | — |
| `estado` | `text` | **NN** | `'ACTIVO'::text` | — |
| `score_origen` | `integer` | sí | — | — |
| `estado_origen` | `text` | **NN** | `''::text` | — |
| `morosidad_origen` | `text` | **NN** | `''::text` | — |
| `dias_vencidos_origen` | `integer` | sí | — | — |
| `saldo_origen` | `numeric(14,2)` | sí | — | — |
| `revision_pendiente` | `boolean` | **NN** | `false` | — |
| `revision_motivo` | `text` | **NN** | `''::text` | — |
| `creado_en` | `timestamptz` | **NN** | `now()` | — |
| `actualizado_en` | `timestamptz` | **NN** | `now()` | — |

### `cargo_cxc`

13 columnas.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `id` | `uuid` | **NN** | `gen_random_uuid()` | **PK** |
| `empresa_id` | `uuid` | **NN** | — | → `empresa.id` |
| `contrato_id` | `uuid` | **NN** | — | → `contrato_cxc.id` |
| `periodo` | `text` | **NN** | — | — |
| `vence_en` | `date` | **NN** | — | — |
| `monto` | `numeric(14,2)` | **NN** | — | — |
| `monto_aplicado` | `numeric(14,2)` | **NN** | `0` | — |
| `estado` | `text` | **NN** | `'ABIERTO'::text` | — |
| `origen` | `text` | **NN** | `'GENERADO'::text` | — |
| `clave_hacienda` | `text` | sí | — | — |
| `nota` | `text` | **NN** | `''::text` | — |
| `creado_en` | `timestamptz` | **NN** | `now()` | — |
| `actualizado_en` | `timestamptz` | **NN** | `now()` | — |

### `cobro_cxc`

25 columnas.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `id` | `uuid` | **NN** | `gen_random_uuid()` | **PK** |
| `empresa_id` | `uuid` | **NN** | — | → `empresa.id` |
| `contrato_id` | `uuid` | sí | — | → `contrato_cxc.id` |
| `consecutivo` | `text` | **NN** | `''::text` | — |
| `fecha_pago` | `date` | **NN** | — | — |
| `fecha_bancaria` | `date` | sí | — | — |
| `fecha_registro` | `date` | sí | — | — |
| `monto` | `numeric(16,2)` | **NN** | — | — |
| `saldo_a_favor` | `numeric(16,2)` | **NN** | `0` | — |
| `forma_pago_id` | `uuid` | sí | — | → `cxc_forma_pago.id` |
| `asociacion_id` | `uuid` | sí | — | → `cxc_asociacion.id` |
| `planilla_id` | `uuid` | sí | — | → `cxc_planilla.id` |
| `referencia` | `text` | **NN** | `''::text` | — |
| `concepto_origen` | `text` | **NN** | `''::text` | — |
| `origen` | `text` | **NN** | `'ARCHIVO'::text` | — |
| `estado` | `text` | **NN** | `'APLICADO'::text` | — |
| `idempotency_key` | `text` | sí | — | — |
| `movimiento_bancario_id` | `uuid` | sí | — | → `movimiento_bancario.id` |
| `reversado_por` | `uuid` | sí | — | → `usuario.id` |
| `reversado_en` | `timestamptz` | sí | — | — |
| `reversa_motivo` | `text` | **NN** | `''::text` | — |
| `creado_por` | `uuid` | sí | — | → `usuario.id` |
| `creado_en` | `timestamptz` | **NN** | `now()` | — |
| `actualizado_en` | `timestamptz` | **NN** | `now()` | — |
| `contrato_origen` | `text` | **NN** | `''::text` | — |

### `cobro_aplicacion`

7 columnas.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `id` | `uuid` | **NN** | `gen_random_uuid()` | **PK** |
| `empresa_id` | `uuid` | **NN** | — | → `empresa.id` |
| `cobro_id` | `uuid` | **NN** | — | → `cobro_cxc.id` |
| `cargo_id` | `uuid` | **NN** | — | → `cargo_cxc.id` |
| `monto` | `numeric(16,2)` | **NN** | — | — |
| `parcial` | `boolean` | **NN** | `false` | — |
| `creado_en` | `timestamptz` | **NN** | `now()` | — |

### `nota_credito_cxc`

14 columnas.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `id` | `uuid` | **NN** | `gen_random_uuid()` | **PK** |
| `empresa_id` | `uuid` | **NN** | — | → `empresa.id` |
| `contrato_id` | `uuid` | **NN** | — | → `contrato_cxc.id` |
| `cargo_id` | `uuid` | sí | — | → `cargo_cxc.id` |
| `fecha` | `date` | **NN** | — | — |
| `monto` | `numeric(16,2)` | **NN** | — | — |
| `motivo` | `text` | **NN** | — | — |
| `estado` | `text` | **NN** | `'APLICADA'::text` | — |
| `creado_por` | `uuid` | sí | — | → `usuario.id` |
| `creado_en` | `timestamptz` | **NN** | `now()` | — |
| `consecutivo` | `bigint` | sí | — | — |
| `anulada_por` | `uuid` | sí | — | → `usuario.id` |
| `anulada_en` | `timestamptz` | sí | — | — |
| `anulacion_motivo` | `text` | **NN** | `''::text` | — |

### `nota_credito_aplicacion`

7 columnas.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `id` | `uuid` | **NN** | `gen_random_uuid()` | **PK** |
| `empresa_id` | `uuid` | **NN** | — | → `empresa.id` |
| `nota_id` | `uuid` | **NN** | — | → `nota_credito_cxc.id` |
| `cargo_id` | `uuid` | **NN** | — | → `cargo_cxc.id` |
| `monto` | `numeric(16,2)` | **NN** | — | — |
| `parcial` | `boolean` | **NN** | `false` | — |
| `creado_en` | `timestamptz` | **NN** | `now()` | — |

### `arreglo_pago_cxc`

23 columnas.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `id` | `uuid` | **NN** | `gen_random_uuid()` | **PK** |
| `empresa_id` | `uuid` | **NN** | — | → `empresa.id` |
| `contrato_id` | `uuid` | **NN** | — | → `contrato_cxc.id` |
| `consecutivo` | `bigint` | **NN** | — | — |
| `saldo_al_pactar` | `numeric(18,2)` | **NN** | — | — |
| `vencido_al_pactar` | `numeric(18,2)` | **NN** | — | — |
| `cuotas_vencidas_al_pactar` | `integer` | **NN** | `0` | — |
| `meses_mora_al_pactar` | `numeric(6,1)` | **NN** | `0` | — |
| `monto_arreglo` | `numeric(18,2)` | **NN** | — | — |
| `plazo_cuotas` | `integer` | **NN** | — | — |
| `prima` | `numeric(18,2)` | **NN** | `0` | — |
| `es_excepcion` | `boolean` | **NN** | `false` | — |
| `autorizado_por` | `uuid` | sí | — | → `usuario.id` |
| `autorizacion_motivo` | `text` | **NN** | `''::text` | — |
| `quebrado_en` | `timestamptz` | sí | — | — |
| `quebrado_por` | `uuid` | sí | — | → `usuario.id` |
| `quebranto_motivo` | `text` | **NN** | `''::text` | — |
| `anulado_en` | `timestamptz` | sí | — | — |
| `anulado_por` | `uuid` | sí | — | → `usuario.id` |
| `anulacion_motivo` | `text` | **NN** | `''::text` | — |
| `observaciones` | `text` | **NN** | `''::text` | — |
| `creado_por` | `uuid` | sí | — | → `usuario.id` |
| `creado_en` | `timestamptz` | **NN** | `now()` | — |

### `arreglo_cuota_cxc`

6 columnas.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `id` | `uuid` | **NN** | `gen_random_uuid()` | **PK** |
| `empresa_id` | `uuid` | **NN** | — | → `empresa.id` |
| `arreglo_id` | `uuid` | **NN** | — | → `arreglo_pago_cxc.id` |
| `numero` | `integer` | **NN** | — | — |
| `vence_en` | `date` | **NN** | — | — |
| `monto` | `numeric(18,2)` | **NN** | — | — |

### `gestion_cxc`

11 columnas.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `id` | `uuid` | **NN** | `gen_random_uuid()` | **PK** |
| `empresa_id` | `uuid` | **NN** | — | → `empresa.id` |
| `contrato_id` | `uuid` | **NN** | — | → `contrato_cxc.id` |
| `usuario_id` | `uuid` | sí | — | → `usuario.id` |
| `canal_id` | `uuid` | **NN** | — | → `cxc_canal_gestion.id` |
| `resultado_id` | `uuid` | **NN** | — | → `cxc_resultado_gestion.id` |
| `notas` | `text` | **NN** | `''::text` | — |
| `saldo_al_gestionar` | `numeric(14,2)` | **NN** | `0` | — |
| `dias_mora_al_gestionar` | `integer` | **NN** | `0` | — |
| `tramo_al_gestionar` | `text` | **NN** | `''::text` | — |
| `gestionada_en` | `timestamptz` | **NN** | `now()` | — |

### `promesa_pago_cxc`

7 columnas.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `id` | `uuid` | **NN** | `gen_random_uuid()` | **PK** |
| `empresa_id` | `uuid` | **NN** | — | → `empresa.id` |
| `gestion_id` | `uuid` | **NN** | — | → `gestion_cxc.id` |
| `contrato_id` | `uuid` | **NN** | — | → `contrato_cxc.id` |
| `fecha_promesa` | `date` | **NN** | — | — |
| `monto` | `numeric(14,2)` | sí | — | — |
| `creado_en` | `timestamptz` | **NN** | `now()` | — |

### `cxc_suspension`

12 columnas.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `id` | `uuid` | **NN** | `gen_random_uuid()` | **PK** |
| `empresa_id` | `uuid` | **NN** | — | → `empresa.id` |
| `contrato_id` | `uuid` | **NN** | — | → `contrato_cxc.id` |
| `cuotas_vencidas` | `integer` | **NN** | — | — |
| `saldo_al_suspender` | `numeric(14,2)` | **NN** | `0` | — |
| `motivo` | `text` | **NN** | — | — |
| `suspendido_por` | `uuid` | sí | — | → `usuario.id` |
| `suspendido_en` | `timestamptz` | **NN** | `now()` | — |
| `reactivado_por` | `uuid` | sí | — | → `usuario.id` |
| `reactivado_en` | `timestamptz` | sí | — | — |
| `reactivacion_motivo` | `text` | **NN** | `''::text` | — |
| `meses_mora` | `numeric(6,1)` | **NN** | `0` | — |

### `cxc_asociacion`

9 columnas.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `id` | `uuid` | **NN** | `gen_random_uuid()` | **PK** |
| `empresa_id` | `uuid` | **NN** | — | → `empresa.id` |
| `nombre` | `text` | **NN** | — | — |
| `patrono` | `text` | sí | — | — |
| `contacto` | `text` | sí | — | — |
| `correo` | `text` | sí | — | — |
| `activa` | `boolean` | **NN** | `true` | — |
| `creado_en` | `timestamptz` | **NN** | `now()` | — |
| `actualizado_en` | `timestamptz` | **NN** | `now()` | — |

### `cxc_planilla`

9 columnas.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `id` | `uuid` | **NN** | `gen_random_uuid()` | **PK** |
| `empresa_id` | `uuid` | **NN** | — | → `empresa.id` |
| `asociacion_id` | `uuid` | **NN** | — | → `cxc_asociacion.id` |
| `referencia` | `text` | **NN** | `''::text` | — |
| `periodo` | `text` | **NN** | `''::text` | — |
| `nota` | `text` | **NN** | `''::text` | — |
| `creado_por` | `uuid` | sí | — | → `usuario.id` |
| `creado_en` | `timestamptz` | **NN** | `now()` | — |
| `actualizado_en` | `timestamptz` | **NN** | `now()` | — |

### `cxc_planilla_movimiento`

5 columnas.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `empresa_id` | `uuid` | **NN** | — | → `empresa.id` |
| `planilla_id` | `uuid` | **NN** | — | **PK** → `cxc_planilla.id` |
| `movimiento_bancario_id` | `uuid` | **NN** | — | **PK** → `movimiento_bancario.id` |
| `vinculado_por` | `uuid` | sí | — | → `usuario.id` |
| `vinculado_en` | `timestamptz` | **NN** | `now()` | — |

### `cxc_importacion`

14 columnas.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `id` | `uuid` | **NN** | `gen_random_uuid()` | **PK** |
| `empresa_id` | `uuid` | **NN** | — | → `empresa.id` |
| `tipo` | `text` | **NN** | — | — |
| `archivo` | `text` | **NN** | `''::text` | — |
| `estado` | `text` | **NN** | `'PREVISUALIZADA'::text` | — |
| `filas` | `integer` | **NN** | `0` | — |
| `nuevos` | `integer` | **NN** | `0` | — |
| `actualizados` | `integer` | **NN** | `0` | — |
| `duplicados` | `integer` | **NN** | `0` | — |
| `cuarentena` | `integer` | **NN** | `0` | — |
| `reporte` | `jsonb` | sí | — | — |
| `creado_por` | `uuid` | sí | — | → `usuario.id` |
| `creado_en` | `timestamptz` | **NN** | `now()` | — |
| `confirmado_en` | `timestamptz` | sí | — | — |

### `cxc_parametro`

6 columnas.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `empresa_id` | `uuid` | **NN** | — | **PK** → `empresa.id` |
| `clave` | `text` | **NN** | — | **PK** |
| `valor` | `text` | **NN** | — | — |
| `descripcion` | `text` | **NN** | `''::text` | — |
| `actualizado_en` | `timestamptz` | **NN** | `now()` | — |
| `actualizado_por` | `uuid` | sí | — | → `usuario.id` |

### `cxc_tramo`

11 columnas.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `id` | `uuid` | **NN** | `gen_random_uuid()` | **PK** |
| `empresa_id` | `uuid` | **NN** | — | → `empresa.id` |
| `codigo` | `text` | **NN** | — | — |
| `etiqueta` | `text` | **NN** | — | — |
| `dias_min` | `integer` | **NN** | — | — |
| `dias_max` | `integer` | **NN** | — | — |
| `orden` | `smallint` | **NN** | — | — |
| `prob_recuperacion` | `numeric(4,2)` | **NN** | — | — |
| `estrategia` | `text` | **NN** | `''::text` | — |
| `canal_sugerido` | `text` | **NN** | `''::text` | — |
| `creado_en` | `timestamptz` | **NN** | `now()` | — |

### `cxc_sede`

9 columnas.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `id` | `uuid` | **NN** | `gen_random_uuid()` | **PK** |
| `empresa_id` | `uuid` | **NN** | — | → `empresa.id` |
| `nombre` | `text` | **NN** | — | — |
| `razon_social` | `text` | sí | — | — |
| `plaza` | `text` | sí | — | — |
| `activa` | `boolean` | **NN** | `true` | — |
| `orden` | `integer` | **NN** | `0` | — |
| `creado_en` | `timestamptz` | **NN** | `now()` | — |
| `actualizado_en` | `timestamptz` | **NN** | `now()` | — |

### `cxc_usuario_sede`

4 columnas.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `empresa_id` | `uuid` | **NN** | — | **PK** → `empresa.id` |
| `usuario_id` | `uuid` | **NN** | — | **PK** → `usuario.id` |
| `sede_id` | `uuid` | **NN** | — | **PK** → `cxc_sede.id` |
| `creado_en` | `timestamptz` | **NN** | `now()` | — |

### `cxc_modalidad`

7 columnas.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `id` | `uuid` | **NN** | `gen_random_uuid()` | **PK** |
| `empresa_id` | `uuid` | **NN** | — | → `empresa.id` |
| `nombre` | `text` | **NN** | — | — |
| `meses_ciclo` | `smallint` | **NN** | `1` | — |
| `quincenal` | `boolean` | **NN** | `false` | — |
| `activa` | `boolean` | **NN** | `true` | — |
| `creado_en` | `timestamptz` | **NN** | `now()` | — |

### `cxc_forma_pago`

8 columnas.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `id` | `uuid` | **NN** | `gen_random_uuid()` | **PK** |
| `empresa_id` | `uuid` | **NN** | — | → `empresa.id` |
| `nombre` | `text` | **NN** | — | — |
| `factor_recuperacion` | `numeric(4,2)` | **NN** | `1.00` | — |
| `es_asociacion` | `boolean` | **NN** | `false` | — |
| `es_domiciliado` | `boolean` | **NN** | `false` | — |
| `activa` | `boolean` | **NN** | `true` | — |
| `creado_en` | `timestamptz` | **NN** | `now()` | — |

### `cxc_canal_gestion`

6 columnas.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `id` | `uuid` | **NN** | `gen_random_uuid()` | **PK** |
| `empresa_id` | `uuid` | **NN** | — | → `empresa.id` |
| `nombre` | `text` | **NN** | — | — |
| `orden` | `smallint` | **NN** | `0` | — |
| `activo` | `boolean` | **NN** | `true` | — |
| `creado_en` | `timestamptz` | **NN** | `now()` | — |

### `cxc_resultado_gestion`

10 columnas.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `id` | `uuid` | **NN** | `gen_random_uuid()` | **PK** |
| `empresa_id` | `uuid` | **NN** | — | → `empresa.id` |
| `codigo` | `text` | **NN** | — | — |
| `etiqueta` | `text` | **NN** | — | — |
| `es_contacto` | `boolean` | **NN** | `true` | — |
| `exige_promesa` | `boolean` | **NN** | `false` | — |
| `dato_malo` | `boolean` | **NN** | `false` | — |
| `orden` | `smallint` | **NN** | `0` | — |
| `activo` | `boolean` | **NN** | `true` | — |
| `creado_en` | `timestamptz` | **NN** | `now()` | — |

## RRHH / Nómina

11 tablas, 170 columnas.

### `empleado`

18 columnas.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `id` | `uuid` | **NN** | `gen_random_uuid()` | **PK** |
| `empresa_id` | `uuid` | **NN** | — | → `empresa.id` |
| `nombre` | `text` | **NN** | — | — |
| `tipo_identificacion` | `text` | **NN** | `'CEDULA'::text` | — |
| `identificacion` | `text` | **NN** | — | — |
| `email` | `text` | sí | — | — |
| `telefono` | `text` | sí | — | — |
| `iban` | `text` | sí | — | — |
| `departamento_id` | `uuid` | sí | — | → `departamento.id` |
| `puesto` | `text` | sí | — | — |
| `fecha_ingreso` | `date` | **NN** | — | — |
| `fecha_salida` | `date` | sí | — | — |
| `salario_base` | `numeric(14,2)` | **NN** | — | — |
| `jornada` | `text` | **NN** | `'MENSUAL'::text` | — |
| `activo` | `boolean` | **NN** | `true` | — |
| `creado_en` | `timestamptz` | **NN** | `now()` | — |
| `hijos` | `integer` | **NN** | `0` | — |
| `conyuge` | `boolean` | **NN** | `false` | — |

### `deduccion_empleado`

12 columnas.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `id` | `uuid` | **NN** | `gen_random_uuid()` | **PK** |
| `empresa_id` | `uuid` | **NN** | — | → `empresa.id` |
| `empleado_id` | `uuid` | **NN** | — | → `empleado.id` |
| `concepto_id` | `uuid` | **NN** | — | → `concepto_nomina.id` |
| `etiqueta` | `text` | **NN** | — | — |
| `cuota` | `numeric(14,2)` | **NN** | — | — |
| `saldo_total` | `numeric(14,2)` | sí | — | — |
| `saldo_restante` | `numeric(14,2)` | sí | — | — |
| `prioridad` | `integer` | **NN** | `100` | — |
| `activo` | `boolean` | **NN** | `true` | — |
| `creado_en` | `timestamptz` | **NN** | `now()` | — |
| `frecuencia` | `text` | **NN** | `'MENSUAL'::text` | — |

### `concepto_nomina`

12 columnas.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `id` | `uuid` | **NN** | `gen_random_uuid()` | **PK** |
| `empresa_id` | `uuid` | **NN** | — | → `empresa.id` |
| `nombre` | `text` | **NN** | — | — |
| `tipo` | `text` | **NN** | — | — |
| `afecta_ccss` | `boolean` | **NN** | `true` | — |
| `afecta_renta` | `boolean` | **NN** | `true` | — |
| `afecta_aguinaldo` | `boolean` | **NN** | `true` | — |
| `base_legal` | `text` | sí | — | — |
| `de_sistema` | `boolean` | **NN** | `false` | — |
| `activo` | `boolean` | **NN** | `true` | — |
| `creado_en` | `timestamptz` | **NN** | `now()` | — |
| `por_horas` | `boolean` | **NN** | `false` | true = la novedad se captura en HORAS y el motor deriva el monto (horas × valor hora × factor, art. 139 CT) |

### `nomina_parametros`

18 columnas.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `id` | `uuid` | **NN** | `gen_random_uuid()` | **PK** |
| `empresa_id` | `uuid` | **NN** | — | → `empresa.id` |
| `anio` | `integer` | **NN** | — | — |
| `cargas` | `jsonb` | **NN** | — | — |
| `tramos_renta` | `jsonb` | **NN** | — | — |
| `ins_riesgos_pct` | `numeric(6,3)` | **NN** | `1.000` | — |
| `aplica_ina` | `boolean` | **NN** | `true` | — |
| `adelanto_pct` | `numeric(5,2)` | **NN** | `50` | — |
| `adelanto_base` | `text` | **NN** | `'SALARIO_BASE'::text` | — |
| `redondeo` | `text` | **NN** | `'COLON'::text` | — |
| `provision_base` | `text` | **NN** | `'REMUNERACION_TOTAL'::text` | — |
| `creado_en` | `timestamptz` | **NN** | `now()` | — |
| `aguinaldo_pct` | `numeric(6,3)` | **NN** | `8.33` | — |
| `vacaciones_pct` | `numeric(6,3)` | **NN** | `4.16` | — |
| `cesantia_pct` | `numeric(6,3)` | **NN** | `1.50` | — |
| `vacaciones_dias_mes` | `numeric(5,2)` | **NN** | `1.00` | — |
| `horas_jornada_mes` | `numeric(6,2)` | **NN** | `240` | — |
| `factor_hora_extra` | `numeric(4,2)` | **NN** | `1.5` | — |

### `corrida_nomina`

22 columnas.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `id` | `uuid` | **NN** | `gen_random_uuid()` | **PK** |
| `empresa_id` | `uuid` | **NN** | — | → `empresa.id` |
| `anio` | `integer` | **NN** | — | — |
| `mes` | `integer` | **NN** | — | — |
| `tipo` | `text` | **NN** | — | — |
| `estado` | `text` | **NN** | `'BORRADOR'::text` | — |
| `fecha_pago` | `date` | **NN** | — | — |
| `parametros` | `jsonb` | **NN** | — | — |
| `total_bruto` | `numeric(14,2)` | **NN** | `0` | — |
| `total_ccss_obrero` | `numeric(14,2)` | **NN** | `0` | — |
| `total_renta` | `numeric(14,2)` | **NN** | `0` | — |
| `total_deducciones` | `numeric(14,2)` | **NN** | `0` | — |
| `total_adelanto` | `numeric(14,2)` | **NN** | `0` | — |
| `total_neto` | `numeric(14,2)` | **NN** | `0` | — |
| `total_patronal` | `numeric(14,2)` | **NN** | `0` | — |
| `total_provisiones` | `numeric(14,2)` | **NN** | `0` | — |
| `creado_por` | `uuid` | sí | — | — |
| `aprobado_por` | `uuid` | sí | — | — |
| `pagado_por` | `uuid` | sí | — | — |
| `creado_en` | `timestamptz` | **NN** | `now()` | — |
| `aprobado_en` | `timestamptz` | sí | — | — |
| `pagado_en` | `timestamptz` | sí | — | — |

### `corrida_linea`

26 columnas.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `id` | `uuid` | **NN** | `gen_random_uuid()` | **PK** |
| `empresa_id` | `uuid` | **NN** | — | → `empresa.id` |
| `corrida_id` | `uuid` | **NN** | — | → `corrida_nomina.id` |
| `empleado_id` | `uuid` | **NN** | — | → `empleado.id` |
| `nombre` | `text` | **NN** | — | — |
| `identificacion` | `text` | **NN** | — | — |
| `iban` | `text` | sí | — | — |
| `departamento` | `text` | sí | — | — |
| `puesto` | `text` | sí | — | — |
| `salario_base` | `numeric(14,2)` | **NN** | — | — |
| `hijos` | `integer` | **NN** | `0` | — |
| `conyuge` | `boolean` | **NN** | `false` | — |
| `bruto` | `numeric(14,2)` | **NN** | `0` | — |
| `base_ccss` | `numeric(14,2)` | **NN** | `0` | — |
| `base_renta` | `numeric(14,2)` | **NN** | `0` | — |
| `ccss_obrero` | `numeric(14,2)` | **NN** | `0` | — |
| `renta` | `numeric(14,2)` | **NN** | `0` | — |
| `deducciones` | `numeric(14,2)` | **NN** | `0` | — |
| `adelanto` | `numeric(14,2)` | **NN** | `0` | — |
| `neto` | `numeric(14,2)` | **NN** | `0` | — |
| `patronal` | `numeric(14,2)` | **NN** | `0` | — |
| `prov_aguinaldo` | `numeric(14,2)` | **NN** | `0` | — |
| `prov_vacaciones` | `numeric(14,2)` | **NN** | `0` | — |
| `prov_cesantia` | `numeric(14,2)` | **NN** | `0` | — |
| `detalle` | `jsonb` | **NN** | `'[]'::jsonb` | — |
| `tratamiento` | `text` | **NN** | `'MENSUAL'::text` | — |

### `corrida_novedad`

7 columnas.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `id` | `uuid` | **NN** | `gen_random_uuid()` | **PK** |
| `empresa_id` | `uuid` | **NN** | — | → `empresa.id` |
| `corrida_id` | `uuid` | **NN** | — | → `corrida_nomina.id` |
| `empleado_id` | `uuid` | **NN** | — | → `empleado.id` |
| `concepto_id` | `uuid` | **NN** | — | → `concepto_nomina.id` |
| `monto` | `numeric(14,2)` | **NN** | — | — |
| `cantidad` | `numeric(8,2)` | sí | — | — |

### `incapacidad`

11 columnas.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `id` | `uuid` | **NN** | `gen_random_uuid()` | **PK** |
| `empresa_id` | `uuid` | **NN** | — | → `empresa.id` |
| `empleado_id` | `uuid` | **NN** | — | → `empleado.id` |
| `entidad` | `text` | **NN** | — | — |
| `fecha_inicio` | `date` | **NN** | — | — |
| `dias` | `integer` | **NN** | — | — |
| `boleta` | `text` | sí | — | — |
| `observaciones` | `text` | sí | — | — |
| `anulada` | `boolean` | **NN** | `false` | — |
| `creado_por` | `uuid` | sí | — | — |
| `creado_en` | `timestamptz` | **NN** | `now()` | — |

### `vacacion`

9 columnas.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `id` | `uuid` | **NN** | `gen_random_uuid()` | **PK** |
| `empresa_id` | `uuid` | **NN** | — | → `empresa.id` |
| `empleado_id` | `uuid` | **NN** | — | → `empleado.id` |
| `fecha_inicio` | `date` | **NN** | — | — |
| `dias` | `numeric(6,2)` | **NN** | — | — |
| `observaciones` | `text` | sí | — | — |
| `anulada` | `boolean` | **NN** | `false` | — |
| `creado_por` | `uuid` | sí | — | — |
| `creado_en` | `timestamptz` | **NN** | `now()` | — |

### `finiquito`

28 columnas.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `id` | `uuid` | **NN** | `gen_random_uuid()` | **PK** |
| `empresa_id` | `uuid` | **NN** | — | → `empresa.id` |
| `empleado_id` | `uuid` | **NN** | — | → `empleado.id` |
| `motivo` | `text` | **NN** | — | — |
| `fecha_salida` | `date` | **NN** | — | — |
| `estado` | `text` | **NN** | `'BORRADOR'::text` | — |
| `dias_vacaciones` | `numeric(6,2)` | **NN** | `0` | — |
| `salario_promedio` | `numeric(14,2)` | **NN** | `0` | — |
| `salario_diario` | `numeric(14,2)` | **NN** | `0` | — |
| `anios_servicio` | `integer` | **NN** | `0` | — |
| `preaviso` | `numeric(14,2)` | **NN** | `0` | — |
| `cesantia` | `numeric(14,2)` | **NN** | `0` | — |
| `vacaciones` | `numeric(14,2)` | **NN** | `0` | — |
| `aguinaldo` | `numeric(14,2)` | **NN** | `0` | — |
| `descuentos` | `numeric(14,2)` | **NN** | `0` | — |
| `total` | `numeric(14,2)` | **NN** | `0` | — |
| `detalle` | `jsonb` | **NN** | `'[]'::jsonb` | — |
| `creado_por` | `uuid` | sí | — | — |
| `aprobado_por` | `uuid` | sí | — | — |
| `pagado_por` | `uuid` | sí | — | — |
| `creado_en` | `timestamptz` | **NN** | `now()` | — |
| `aprobado_en` | `timestamptz` | sí | — | — |
| `pagado_en` | `timestamptz` | sí | — | — |
| `base_ccss` | `numeric(14,2)` | **NN** | `0` | — |
| `ccss_obrero` | `numeric(14,2)` | **NN** | `0` | — |
| `renta` | `numeric(14,2)` | **NN** | `0` | — |
| `dias_vacaciones_manual` | `boolean` | **NN** | `true` | — |
| `patronal` | `numeric(14,2)` | **NN** | `0` | — |

### `nomina_archivo_pago`

8 columnas.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `id` | `uuid` | **NN** | `gen_random_uuid()` | **PK** |
| `empresa_id` | `uuid` | **NN** | — | → `empresa.id` |
| `corrida_id` | `uuid` | **NN** | — | → `corrida_nomina.id` |
| `consecutivo` | `integer` | **NN** | — | — |
| `registros` | `integer` | **NN** | — | — |
| `total` | `numeric(14,2)` | **NN** | — | — |
| `creado_por` | `uuid` | sí | — | — |
| `creado_en` | `timestamptz` | **NN** | `now()` | — |

## Inventario

8 tablas, 83 columnas.

Cofres, urnas y suministros por sede. **La existencia nunca se guarda en un campo: se deriva.**
Para los artículos de modo UNIDAD sale de las fichas de `inv_unidad` (su estado y su sede);
para los de modo CANTIDAD, de sumar `inv_movimiento`. Así no hay ningún contador que alguien
tenga que acordarse de actualizar, que es lo que se desincroniza.

### `inv_categoria`

7 columnas. Categorías del catálogo, con un nivel de subcategoría (`padre_id`). La unicidad del nombre son DOS índices parciales, no un UNIQUE: con `padre_id` NULL, un UNIQUE no impide repetir el nombre entre las categorías raíz (en PostgreSQL NULL nunca es igual a NULL).

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `id` | `uuid` | **NN** | `gen_random_uuid()` | **PK** |
| `empresa_id` | `uuid` | **NN** | — | → `empresa.id` |
| `padre_id` | `uuid` | sí | — | → `inv_categoria.id` |
| `nombre` | `text` | **NN** | — | — |
| `activo` | `boolean` | **NN** | `true` | — |
| `orden` | `integer` | **NN** | `0` | — |
| `creado_en` | `timestamptz` | **NN** | `now()` | — |

### `inv_articulo`

14 columnas. El catálogo. `modo_control` es la decisión central del módulo: **UNIDAD** = cada objeto físico es una fila de `inv_unidad` (cofres); **CANTIDAD** = solo se cuenta (urnas, suministros). No se cambia con historia encima.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `id` | `uuid` | **NN** | `gen_random_uuid()` | **PK** |
| `empresa_id` | `uuid` | **NN** | — | → `empresa.id`, único |
| `categoria_id` | `uuid` | **NN** | — | → `inv_categoria.id` |
| `codigo` | `text` | **NN** | — | único |
| `nombre` | `text` | **NN** | — | — |
| `modo_control` | `text` | **NN** | — | — |
| `unidad_medida` | `text` | **NN** | `'unidad'::text` | — |
| `proveedor_id` | `uuid` | sí | — | → `proveedor.id` |
| `clasificacion_id` | `uuid` | sí | — | — |
| `activo` | `boolean` | **NN** | `true` | — |
| `nota` | `text` | sí | — | — |
| `creado_en` | `timestamptz` | **NN** | `now()` | — |
| `actualizado_en` | `timestamptz` | **NN** | `now()` | — |
| `creado_por` | `uuid` | sí | — | → `usuario.id` |

### `inv_nivel`

6 columnas. Mínimo y máximo por artículo Y SEDE. Va en su propia tabla porque el mínimo de una urna en Cartago no tiene relación con el de Sabana. Mínimo 0 = sin nivel definido: entonces no hay semáforo ni sugerencia de pedido.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `empresa_id` | `uuid` | **NN** | — | → `empresa.id`, **PK** |
| `articulo_id` | `uuid` | **NN** | — | → `inv_articulo.id`, **PK** |
| `sede_id` | `uuid` | **NN** | — | → `sede.id`, **PK** |
| `minimo` | `integer` | **NN** | `0` | — |
| `maximo` | `integer` | **NN** | `0` | — |
| `actualizado_en` | `timestamptz` | **NN** | `now()` | — |

### `inv_unidad`

14 columnas. Una fila por objeto físico (solo para artículos de modo UNIDAD). La fila ES el objeto: su `sede_id` y `estado` son la existencia, sin contador que mantener. `sede_id` en NULL solo mientras el estado es EN_TRANSITO: salió del origen y nadie la recibió, así que no cuenta en ninguna de las dos sedes.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `id` | `uuid` | **NN** | `gen_random_uuid()` | **PK** |
| `empresa_id` | `uuid` | **NN** | — | → `empresa.id`, único |
| `articulo_id` | `uuid` | **NN** | — | → `inv_articulo.id` |
| `numero` | `text` | **NN** | — | único |
| `sede_id` | `uuid` | sí | — | → `sede.id` |
| `sede_destino_id` | `uuid` | sí | — | → `sede.id` |
| `estado` | `text` | **NN** | `'DISPONIBLE'::text` | 8 valores · solo DISPONIBLE, RESERVADA y EXHIBICION son existencia |
| `costo_crc` | `numeric(14,2)` | **NN** | `0` | — |
| `es_consignada` | `boolean` | **NN** | `false` | el capital es del proveedor · solo aplica a modo UNIDAD · obliga `proveedor_id` |
| `proveedor_id` | `uuid` | sí | — | → `proveedor.id` · obligatorio si `es_consignada` |
| `documento_cxp_id` | `uuid` | sí | — | → `documento_cxp.id` · la factura con la que se COMPRÓ |
| `cxp_consignacion_id` | `uuid` | sí | — | → `documento_cxp.id` · la que NACIÓ al usarla (mig 0071) |
| `ingresada_en` | `date` | **NN** | — | — |
| `creado_en` | `timestamptz` | **NN** | `now()` | — |
| `actualizado_en` | `timestamptz` | **NN** | `now()` | — |

**Las dos columnas de CxP son dos hechos distintos y por eso son dos columnas.** `documento_cxp_id`
es «la factura con la que compré esta unidad»; `cxp_consignacion_id` es «la factura que se generó
porque la usé». Sobrecargar la primera con los dos sentidos es el mismo defecto que corrigió la
migración 0070 con el estado `NO_APARECIO`. El índice único parcial `uq_inv_unidad_cxp_consignacion`
(WHERE NOT NULL, porque en Postgres NULL ≠ NULL y el caso normal es no tener factura) impide que una
unidad genere dos cuentas por pagar. El estado «pendiente de pago» se DERIVA de esa columna: no hay
ninguna marca de «pagado», así que anular la factura en CxP devuelve la unidad sola a la cola.

### `inv_servicio`

10 columnas. El funeral prestado: el hecho que DESCARGA el inventario. Antes de este módulo no se registraba en ninguna parte del sistema (CxC lleva la cartera de asociados, que es otra cosa).

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `id` | `uuid` | **NN** | `gen_random_uuid()` | **PK** |
| `empresa_id` | `uuid` | **NN** | — | → `empresa.id`, único |
| `numero` | `text` | **NN** | — | único |
| `sede_id` | `uuid` | sí | — | → `sede.id` |
| `fecha` | `date` | **NN** | — | — |
| `a_nombre_de` | `text` | **NN** | `''::text` | — |
| `contrato_id` | `uuid` | sí | — | → `contrato_cxc.id` |
| `nota` | `text` | sí | — | — |
| `creado_en` | `timestamptz` | **NN** | `now()` | — |
| `creado_por` | `uuid` | sí | — | → `usuario.id` |

### `inv_movimiento`

17 columnas. El libro mayor del inventario, append-only. Toda existencia se deriva de acá. `cantidad` es SIEMPRE positiva y el sentido lo da el `tipo`: guardar negativos obligaría a que cada consulta recordara el signo. Por eso el ajuste son dos tipos (AJUSTE_MAS / AJUSTE_MENOS).

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `id` | `uuid` | **NN** | `gen_random_uuid()` | **PK** |
| `empresa_id` | `uuid` | **NN** | — | → `empresa.id` |
| `articulo_id` | `uuid` | **NN** | — | → `inv_articulo.id` |
| `unidad_id` | `uuid` | sí | — | → `inv_unidad.id` |
| `tipo` | `text` | **NN** | — | — |
| `cantidad` | `integer` | **NN** | — | — |
| `sede_id` | `uuid` | sí | — | → `sede.id` |
| `sede_contra_id` | `uuid` | sí | — | → `sede.id` |
| `costo_unitario_crc` | `numeric(14,2)` | **NN** | `0` | — |
| `fecha` | `date` | **NN** | — | — |
| `servicio_id` | `uuid` | sí | — | → `inv_servicio.id` |
| `proveedor_id` | `uuid` | sí | — | → `proveedor.id` |
| `documento_cxp_id` | `uuid` | sí | — | → `documento_cxp.id` |
| `traslado_id` | `uuid` | sí | — | — |
| `motivo` | `text` | sí | — | — |
| `creado_en` | `timestamptz` | **NN** | `now()` | — |
| `creado_por` | `uuid` | sí | — | → `usuario.id` |

### `inv_traslado`

12 columnas. Envío entre sedes, con estado propio: sale, viaja y alguien lo recibe. `traslado_id` en `inv_movimiento` une la salida con su entrada. Sin el paso intermedio, una unidad extraviada en el camino desaparecería del sistema sin que nadie responda por ella.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `id` | `uuid` | **NN** | `gen_random_uuid()` | **PK** |
| `empresa_id` | `uuid` | **NN** | — | → `empresa.id`, único |
| `numero` | `text` | **NN** | — | único |
| `sede_origen_id` | `uuid` | **NN** | — | → `sede.id` |
| `sede_destino_id` | `uuid` | **NN** | — | → `sede.id` |
| `estado` | `text` | **NN** | `'EN_TRANSITO'::text` | — |
| `enviado_en` | `date` | **NN** | — | — |
| `recibido_en` | `date` | sí | — | — |
| `enviado_por` | `uuid` | sí | — | → `usuario.id` |
| `recibido_por` | `uuid` | sí | — | → `usuario.id` |
| `nota` | `text` | sí | — | — |
| `creado_en` | `timestamptz` | **NN** | `now()` | — |

### `inv_conteo`

12 columnas. La hoja de un conteo cíclico: una sede, opcionalmente una categoría, y un estado. Solo puede haber UNA hoja abierta por sede a la vez (índice único parcial `uq_inv_conteo_abierto_por_sede`), porque dos hojas simultáneas sobre la misma bodega producirían dos verdades y ajustes que se pisan. Anular existe para no dejar una sede bloqueada por una hoja que nadie va a terminar: forzar el cierre generaría ajustes falsos.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `id` | `uuid` | **NN** | `gen_random_uuid()` | **PK** |
| `empresa_id` | `uuid` | **NN** | — | → `empresa.id`, único |
| `numero` | `text` | **NN** | — | único · `CT-AAAA-NNNN` |
| `sede_id` | `uuid` | **NN** | — | → `sede.id` |
| `categoria_id` | `uuid` | sí | — | → `inv_categoria.id` · NULL = toda la sede |
| `estado` | `text` | **NN** | `'ABIERTO'::text` | ABIERTO · CERRADO · ANULADO |
| `abierto_en` | `date` | **NN** | — | — |
| `cerrado_en` | `date` | sí | — | CHECK: existe si y solo si está CERRADO |
| `abierto_por` | `uuid` | sí | — | → `usuario.id` |
| `cerrado_por` | `uuid` | sí | — | → `usuario.id` |
| `nota` | `text` | sí | — | al anular se le concatena el motivo |
| `creado_en` | `timestamptz` | **NN** | `now()` | — |

### `inv_conteo_linea`

10 columnas. Una fila por cosa a contar: por artículo si se cuenta por cantidad, por unidad física si se cuenta por unidad. `cantidad_sistema` es la FOTO congelada al abrir la hoja —no se recalcula—, así que un movimiento registrado mientras alguien cuenta no altera la comparación. `cantidad_contada` en NULL significa «nadie la miró todavía», que es distinto de contar cero: por eso es nullable y no `DEFAULT 0`. El cierre exige que ninguna quede en NULL y que toda diferencia traiga `motivo`.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `id` | `uuid` | **NN** | `gen_random_uuid()` | **PK** |
| `empresa_id` | `uuid` | **NN** | — | → `empresa.id`, único |
| `conteo_id` | `uuid` | **NN** | — | → `inv_conteo.id` ON DELETE CASCADE |
| `articulo_id` | `uuid` | **NN** | — | → `inv_articulo.id` |
| `unidad_id` | `uuid` | sí | — | → `inv_unidad.id` · NULL = la línea es de cantidad |
| `cantidad_sistema` | `integer` | **NN** | — | congelada al abrir |
| `cantidad_contada` | `integer` | sí | — | NULL = sin contar (≠ contar 0) |
| `motivo` | `text` | sí | — | obligatorio si hay diferencia |
| `contado_en` | `timestamptz` | sí | — | — |
| `contado_por` | `uuid` | sí | — | → `usuario.id` |

Dos índices únicos, no uno: `(empresa_id, conteo_id, articulo_id, unidad_id)` para las líneas de unidad y el parcial `uq_inv_conteo_linea_articulo … WHERE unidad_id IS NULL` para las de cantidad. En Postgres `NULL ≠ NULL`, así que sin el parcial se podrían cargar dos líneas del mismo artículo en la misma hoja.

### `inv_consecutivo`

3 columnas. Consecutivos por empresa y ámbito (SERVICIO, TRASLADO, CONTEO, o el código del artículo cuyas unidades se numeran). Se toma con un solo `INSERT … ON CONFLICT … RETURNING`, así que dos operaciones simultáneas no pueden llevarse el mismo número.

| Columna | Tipo | Nulo | Default | Notas |
|---|---|---|---|---|
| `empresa_id` | `uuid` | **NN** | — | → `empresa.id`, **PK** |
| `ambito` | `text` | **NN** | — | **PK** |
| `siguiente` | `integer` | **NN** | `1` | — |

