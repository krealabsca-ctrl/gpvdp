# Ingesta de facturas electrónicas — Especificación técnica v1.0

**Estado:** diseño. Las decisiones marcadas **[PENDIENTE]** bloquean la construcción de su parte.
**Fecha:** 9 de setiembre de 2026 · **Migración que estrena:** 0081 · **Módulo:** `internal/cxp`

---

## 1. El problema, medido

Hoy la facturación electrónica entra así:

```
correo del proveedor → Apps Script (1 por buzón) → Google Sheets → .xlsx a mano → importador de CxP
```

Tres cosas rompen en esa cadena, y las tres están verificadas:

1. **El script no sabe de qué empresa es la factura.** Nunca lee el nodo `Receptor` del XML. Por eso hace
   falta un script y un buzón por empresa, y la empresa queda determinada por *dónde llegó el correo*, no
   por lo que dice el comprobante.
2. **El PDF nunca se captura.** El script busca `filename:xml`; el PDF se queda en el correo y hay que
   bajarlo a mano cada vez que alguien quiere ver la factura.
3. **El XML se degrada a Excel.** El XML trae el dato exacto —montos, moneda, tipo de cambio, condición de
   venta, plazo, cédula del emisor— y se convierte en texto formateado de una celda. El bug de producción
   del 9 de setiembre de 2026 salió justo de ahí: la fecha `dd/mm/aaaa` de la celda entraba invertida.

El XML es la fuente de la verdad y hoy se tira a la basura después de usarlo.

---

## 2. Lo que ya está decidido (por el Director Financiero)

| # | Decisión | Fecha |
|---|---|---|
| 1 | La factura entra directo al ERP, sin Sheets ni Excel intermedios | 2026-09-08 |
| 2 | Se captura el PDF además del XML, para no bajarlo del correo | 2026-09-08 |
| 3 | El sistema determina a qué empresa pertenece la factura | 2026-09-08 |
| 4 | Existe una cola de errores de recepción | 2026-09-08 |
| 5 | Hay una pantalla de configuración de fuentes de recepción por empresa | 2026-09-08 |
| 6 | Un buzón de correo distinto por empresa | 2026-09-09 |
| 7 | Factun (API) es una tercera etapa: mínimo 6 meses. Hay que operar antes | 2026-09-09 |
| 8 | El tipo de cambio **de la factura** manda para lo que se le debe al proveedor | 2026-09-07 |
| 9 | Las notas de crédito son una propuesta aparte, **todavía sin aprobar** | 2026-09-08 |
| 10 | **Solo `FacturaElectronica` crea cuenta por pagar.** Los otros 6 tipos se guardan visibles y contados, con su XML, sin generar deuda | 2026-09-09 |
| 11 | **Vencimiento = fecha de emisión + `PlazoCredito` del XML.** Manda lo que el proveedor declaró en el comprobante | 2026-09-09 |
| 12 | **La factura de contado nace bloqueada para pago**, con motivo «contado: confirmar si ya se pagó». Entra al expediente y se ve, pero no puede salir al archivo del banco sin que una persona la libere | 2026-09-09 |
| 13 | **La ingesta corre con un usuario técnico por integración** («Recepción de facturas»), sin contraseña utilizable y sin acceso a la app, para que la auditoría diga que lo creó la máquina y no una persona | 2026-09-09 |

---

## 3. Decisiones de arquitectura (mías, con su fundamento)

### D1 · El parser de XML vive al lado del de Excel, no en un módulo nuevo

`parsearFacturasXML(data []byte) ([]FilaImportada, ResumenImportacion, error)` en `internal/cxp/`, con el
**mismo contrato** que `parsearFacturas`. Con eso, todo lo que viene después ya existe y no se reescribe:

- deduplicación por clave contra `ClavesExistentes` y contra el propio archivo;
- alta/resolución de proveedor por cédula (`resolverProveedor`), con aprendizaje de condición de pago;
- `filaAInput` → `DocumentoInput` → `Service.CrearDocumento`, que ya calcula `total_crc`, ya exige la clave
  para tipo CXP, ya detecta el duplicado por el UNIQUE y ya escribe el evento `CREAR_DOCUMENTO`;
- acumulación de errores fila por fila, aislamiento por empresa y el permiso `cxp.importar`.

**Por qué no un módulo nuevo con puerto.** El precedente `inventario.FacturadorCxP` existe porque
inventario es *otro dominio* que necesita provisionar en CxP. Recibir facturas de proveedor **es** cuentas
por pagar: un puerto ahí compraría aislamiento que no hace falta y costaría un adaptador con traducción de
errores — que es exactamente donde este repo se equivocó cinco veces (el comentario de `errorDeCxP` lo
dice).

### D2 · Dos puertas sobre el mismo parser, en dos etapas

Es el idiom que ya usa Bancos: misma tabla, dos puertas, un permiso por puerta.

**Puerta 1 — carga manual de XML (etapa 1).** La pantalla *Importar* que ya existe acepta archivos `.xml`
(y un `.zip` de varios) además del `.xlsx`. El service elige parser por la forma del archivo: `PK` al
inicio = zip/xlsx, `<?xml` o `<` = XML. Cero superficie de autenticación nueva.

> Esto es lo que permite **dejar de usar Sheets ya**, y hace que el parser se pruebe contra facturas reales
> antes de que la automatización se construya encima.

**Puerta 2 — recepción automática (etapa 2).** `POST /v1/cxp/recepcion` con token de máquina; el correo
aterriza en `cxp_recepcion`, pasa por el mismo parser y sigue el mismo camino. El Apps Script se reescribe
para hacer POST en vez de escribir en Sheets.

### D3 · La fuente de recepción **es** la credencial

No hay tabla de tokens aparte. `cxp_fuente_recepcion` = `(empresa_id, correo, token_hash, activo)`.
**Una fila = un buzón = una empresa = una credencial.** Calza exacto con la decisión 6 y no agrega un
concepto que después haya que administrar por separado.

### D4 · La ruta de máquina la autoriza su middleware, NO la matriz RBAC

Verificado en el código: `RequirePermiso` lee `claims.Rol` y consulta la matriz; con rol vacío devuelve
0 filas → **403 siempre**. Y peor: con `empresa_id` vacío, `rbac/repository.go` castea `$1::uuid` y
Postgres revienta → **500, no 403**. Así que `P("...")` no puede custodiar esta ruta.

**Un token = un endpoint.** El middleware es el único autorizador, y eso queda escrito en el código.
Lo que **no** se hace es sintetizar un código de rol para el token: ese es exactamente el bug de rol
hardcodeado que este repo ya cometió tres veces (`bancos.ver`, `MatrizDefault`, `cxp/masivo.go`) y que la
migración 0079 vino a arreglar.

El `empresa_id` sale **de la fila del token**, nunca del body, query, header o path. El lookup replica el
`AND e.activo = true` que ya llevan todas las resoluciones de empresa en `internal/auth`, para que
desactivar una empresa deje de hacerla escribible.

### D5 · La fecha de emisión sale de la clave numérica

Ya construido el 2026-09-09. La clave de 50 dígitos trae `ddmmaa` en las posiciones 4-9, sin ambigüedad de
orden **ni de zona horaria**. Verificado que hace falta: `fechaISO("2026-08-14T05:41:07Z")` da 14 de agosto
cuando ese instante es el **13** a las 23:41 en Costa Rica. Y contra los 4.526 documentos ya importados, la
fecha de la clave coincidió con la registrada en 4.526 casos: cero discrepancias.

### D6 · La aritmética se verifica y, si no cierra, la fila se rechaza

Esta es la decisión menos obvia y la más importante. `encoding/xml` **no tiene** equivalente de
`DisallowUnknownFields`: un elemento renombrado entre versiones —o con un typo en el struct— devuelve
**cero, sin error y sin ruido**. Verificado: `xml:"ResumenFactura>TotalComprovante"` (typo a propósito)
devuelve `""` con `err=nil`. Y como `decOrZero` convierte `""` en 0 sin quejarse, un `TotalImpuesto`
renombrado en una versión futura produciría facturas **con IVA 0 que pasan todos los tests**.

Por eso, antes de crear el documento se verifica que `subtotal + impuesto − descuento` cuadre con el total,
y si no cuadra la fila se rechaza **diciendo qué campo vino vacío**. Es el mismo espíritu de
`FechasIlegiblesError` en Bancos: cuando el parser es el único que sabe que el formato cambió, tiene que
gritarlo, no devolver 0.

---

## 4. Reglas del parser de XML (todas verificadas ejecutando código)

Estas no son preferencias de estilo: cada una corresponde a un modo de falla que se probó.

| Regla | Qué pasa si no se sigue |
|---|---|
| **Ningún struct tag lleva URI de namespace.** Nunca. | Un tag con el namespace de la v4.3 contra un documento v4.4 da **error duro**: la ingesta reventaría el día que Hacienda suba de versión. Sin namespace, el match es por nombre local y un solo struct sirve para 4.2, 4.3, 4.4, con prefijo (`<fe:Clave>`) y sin `xmlns`. |
| **Nunca repetir el namespace en los segmentos de una ruta `A>B`.** | `xml:"NS ResumenFactura>NS TotalComprobante"` devuelve **`""` con `err=nil`**. Es la forma que un programador escribiría por intuición, y da total cero sin una sola queja. |
| **`XMLName xml.Name` con el tipo en el tag, y lista blanca de tipos.** | Un struct sin `XMLName` acepta **cualquier** raíz: una **nota de crédito se registraría como factura por pagar**. Una NC resta; entraría sumando. |
| **Todo lo que puede repetirse va a slice.** | Un elemento repetido en un campo escalar deja **el último, sin error**: dos `LineaDetalle` de 1000 y 2000 dan "2000", que parece un total válido. |
| **Montos: campo `string` → `TrimSpace`/`limpiarNumero` → `decimal.NewFromString`.** | `decimal.Decimal` como tipo de campo XML compila y funciona en una línea, y se cae con **todo el documento** en cuanto llega un XML con sangría: `<TotalComprobante>\n 3390.00\n </...>` → «can't convert to decimal». Y `float64` funciona perfecto — por eso es la trampa más fácil: está prohibido por CLAUDE.md y nada en el compilador lo impide. |
| **`xml.NewDecoder` en bucle hasta `io.EOF`, nunca `xml.Unmarshal`.** | `Unmarshal` lee **solo el primer comprobante** de un archivo con varios, en silencio. |
| **`Decoder.CharsetReader` obligatorio.** | Un XML declarado `ISO-8859-1` —Hacienda emite algunos así— falla entero: «encoding "ISO-8859-1" declared but Decoder.CharsetReader is nil». Con el reader puesto, "PANADERÍA SEÑOR" entra bien. |
| **El XML crudo se guarda como `[]byte` original.** | `xml:"Signature,innerxml"` **no compila a runtime**; `,innerxml` solo funciona sin nombre de elemento. |
| **`io.LimitReader` en la lectura del archivo.** | `leerArchivo` hoy hace `io.ReadAll` sin tope. Un XML lo sube un tercero. `bccr.go` ya usa `io.LimitReader(..., 1<<20)` como precedente. |

### Lo que **no** hay que defender (para no gastar código de más)

Verificado con las tres variantes de cada caso:

- **XXE y billion-laughs son estructuralmente imposibles.** `encoding/xml` no procesa declaraciones de
  entidad de DTD: cualquier entidad propia —interna, externa `SYSTEM file:///etc/passwd`, o anidada— da
  «XML syntax error: invalid character entity». No hace falta mitigación porque el stack no tiene el hueco.
- **El BOM UTF-8 se parsea bien**, no hay que quitarlo.
- **Un documento truncado da error claro** («unexpected EOF»).
- **Prefijo vs `xmlns` por defecto es indistinguible para Go**: `xml.Name.Space` siempre trae la URI.
- **La firma XAdES incrustada no interfiere** con las rutas ancladas en hijos directos de la raíz.
  (Sí interferiría con un tag suelto: `<Total>111</Total>` seguido de `<ds:Total>99999</ds:Total>` leído por
  `xml:"Total"` devuelve **99999**. Otra razón para la verificación aritmética de D6.)

---

## 5. Esquema de datos (migración 0081)

```sql
-- Las cédulas jurídicas que responde cada empresa. Es una LISTA, no una columna: «Valle de Paz»
-- agrupa 6 titulares legales (Jardines, Colinas, Religiosa, Privado de Cartago, COPENAE), según
-- CLAUDE.md § Parámetros de negocio. Con una sola cédula, la mayoría de las facturas de la empresa
-- más grande caería en la cola de errores el primer día.
CREATE TABLE empresa_cedula (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,
    cedula     text NOT NULL,
    titular    text,                    -- «Jardines del Recuerdo», para que se entienda al leerlo
    creado_en  timestamptz NOT NULL DEFAULT now(),
    -- UNIQUE GLOBAL: una cédula no puede responder a dos empresas, o el cotejo sería ambiguo.
    UNIQUE (cedula),
    CONSTRAINT empresa_cedula_formato CHECK (cedula ~ '^[0-9]{9,12}$')
);

-- La fuente de recepción ES la credencial (D3).
CREATE TABLE cxp_fuente_recepcion (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id   uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,
    correo       text NOT NULL,
    token_hash   text NOT NULL,          -- sha256 hex; el claro se muestra UNA vez
    activo       boolean NOT NULL DEFAULT true,
    ultimo_uso   timestamptz,
    creado_por   uuid REFERENCES usuario(id),
    creado_en    timestamptz NOT NULL DEFAULT now(),
    UNIQUE (token_hash),                 -- el índice ES el mecanismo de búsqueda por digest
    UNIQUE (empresa_id, correo)
);

-- La tabla de aterrizaje: lo recibido se guarda ANTES de intentar interpretarlo.
CREATE TABLE cxp_recepcion (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id     uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,
    fuente_id      uuid REFERENCES cxp_fuente_recepcion(id),
    -- Idempotencia: derivada del origen, nunca aleatoria.
    idempotency_key text NOT NULL,       -- message_id + '|' + sha256(xml)
    -- El comprobante
    clave          text,                 -- la de Hacienda, si se pudo leer
    tipo_documento text,                 -- XMLName.Local: FacturaElectronica, NotaCreditoElectronica, ...
    version_schema text,                 -- del XMLName.Space: v4.2 / v4.3 / v4.4
    xml_crudo      bytea NOT NULL,       -- el original, tal como llegó
    pdf            bytea,                -- el adjunto, si vino
    pdf_filename   text,
    -- Trazabilidad al correo
    message_id     text,
    asunto         text,
    remitente      text,
    recibido_en    timestamptz,
    -- Resultado
    estado         text NOT NULL DEFAULT 'PENDIENTE',
    motivo         text,                 -- por qué quedó parqueada, en palabras
    documento_id   uuid REFERENCES documento_cxp(id),
    creado_en      timestamptz NOT NULL DEFAULT now(),
    procesado_en   timestamptz,
    CONSTRAINT cxp_recepcion_estado_check CHECK (estado IN
        ('PENDIENTE','PROCESADA','DUPLICADA','PARQUEADA','DESCARTADA'))
);

-- El WHERE es obligatorio: NULL nunca es igual a NULL (misma lección que 0066, 0067, 0069 y 0071).
CREATE UNIQUE INDEX idx_cxp_recepcion_idem ON cxp_recepcion (empresa_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL AND idempotency_key <> '';
CREATE INDEX idx_cxp_recepcion_estado ON cxp_recepcion (empresa_id, estado, creado_en DESC);
```

**Por qué la llave de idempotencia va en `cxp_recepcion` y no en `documento_cxp`:** `esViolacionUnica`
traga *cualquier* 23505 y lo reporta como `ErrDocumentoDuplicado`. Hoy es correcto porque
`(empresa_id, clave)` es el único UNIQUE de la tabla; agregarle un segundo índice único haría que ese
choque se le muestre al usuario como «ya existe un documento con esa clave», que sería falso.

**Por qué `comprobante_pago` no sirve para el XML/PDF:** es `UNIQUE (documento_id)` —un solo archivo, el
segundo reemplaza al primero— y su INSERT solo procede si el documento está en `PAGADO` o `CONCILIADO`.

---

## 6. Los estados de una recepción

```
                    ┌─────────────┐
  POST del script → │ PENDIENTE   │
                    └──────┬──────┘
                           │ se parsea y se cotejan Receptor / tipo / aritmética
        ┌──────────────────┼──────────────────┬─────────────────────┐
        ▼                  ▼                  ▼                     ▼
  ┌───────────┐     ┌─────────────┐    ┌────────────┐       ┌─────────────┐
  │ PROCESADA │     │  DUPLICADA  │    │ PARQUEADA  │       │ DESCARTADA  │
  │ documento │     │ la clave ya │    │ hay que    │       │ no es un    │
  │ creado    │     │ existía →   │    │ arreglar   │       │ comprobante │
  │           │     │ se enlaza   │    │ algo       │       │ por pagar   │
  └───────────┘     └─────────────┘    └─────┬──────┘       └─────────────┘
                                             │ reintentar (acción de la bandeja)
                                             └──────────► vuelve a PENDIENTE
```

**Nada se pierde y nada se borra.** `PARQUEADA` es la cola de errores de la decisión 4, con el motivo
escrito en palabras: «el Receptor 3101999999 no corresponde a ninguna empresa configurada», «factura en
EUR: el sistema maneja CRC y USD», «la aritmética no cuadra: TotalImpuesto vino vacío». `DESCARTADA` es
para lo que legítimamente no es una cuenta por pagar (un `MensajeReceptor`, un recibo electrónico de pago),
y **conserva el XML** para que la etapa de notas de crédito tenga su materia prima cuando se apruebe.

---

## 7. Cotejo del Receptor — el mecanismo antiempresa-equivocada

`UNIQUE (empresa_id, clave)` significa que la misma factura *puede* existir en dos empresas. El cotejo del
`Receptor` no es validación de lujo: **es lo único que impide que una factura de Coopeprofa aterrice en
Valle de Paz.**

Tres candados independientes:

1. **El token** dice de qué empresa es el buzón (`cxp_fuente_recepcion.empresa_id`).
2. **El buzón** que reporta el script se compara contra `cxp_fuente_recepcion.correo`. Sin esto el campo
   `correo` de la pantalla sería **decorativo**: con el token de Valle de Paz pegado por error en el script
   de Coopeprofa, todas las facturas de Coopeprofa entrarían en Valle de Paz mientras la pantalla afirma lo
   contrario. Es el modo de falla más probable de todo el diseño (copy-paste entre scripts).
3. **El XML** dice a quién se le facturó (`Receptor > Identificacion > Numero`), contra `empresa_cedula`.

### El cotejo se escribe con ramas explícitas, nunca con un `==`

Esto no es pedantería: `if receptor != emp.Cedula { parquear }` con **los dos vacíos** —un
`TiqueteElectronico` no trae `Receptor`, y una empresa sin cédulas cargadas escanea a `""`— da
`"" == ""` ⇒ **calza**, y el comprobante entra en esa empresa sin que nada lo cuestione. Es la misma
lección que ya está escrita en `bancos/repository_clasif.go:82-97`: **un alcance vacío tiene que cerrar,
no abrir.**

| Caso | Resultado |
|---|---|
| El `Receptor` está en `empresa_cedula` de la empresa del token | se procesa |
| **La empresa no tiene ninguna cédula cargada** | **PARQUEADA** con motivo «esta empresa no tiene cédulas configuradas» — y **ni se compara** |
| **El XML no trae `Receptor` o viene vacío** | **PARQUEADA** con motivo «el comprobante no dice a quién va» |
| El `Receptor` pertenece a otra empresa del grupo | **PARQUEADA** — nunca se reencamina sola: eso sería la ingesta decidiendo a qué empresa cargarle un gasto, y sería la primera superficie de **escritura** que cruza empresas (hoy solo `internal/grupo` cruza, y solo lee) |
| El `Receptor` no es ninguna cédula conocida | **PARQUEADA** con la cédula recibida y la esperada en el motivo |
| El buzón reportado no calza con el de la fuente | **PARQUEADA** con motivo «el token no corresponde a este buzón» |
| La empresa está inactiva | el token no resuelve → 401 (el lookup lleva `AND e.activo = true`) |

**Test obligatorio**, en el molde de Bancos: tabla de casos con `(receptor="", cedula="")`,
`(receptor="", cedula="310…")` y `(receptor="310…", cedula="")` — los tres tienen que dar PARQUEADO.

### Las dos puertas se comportan distinto ante la falta de cédulas, y es a propósito

| Puerta | Sin cédulas configuradas |
|---|---|
| **Máquina** (`POST /v1/cxp/recepcion`) | **Fail-closed:** rechaza toda recepción. Nadie está mirando, así que la única opción segura es no aceptar |
| **Manual** (pantalla *Importar*) | **Avisa fuerte y deja seguir:** el preview dice «no se puede verificar el destinatario: esta empresa no tiene cédulas configuradas» y muestra la cédula del `Receptor` de cada factura. Hay una persona leyendo el preview antes de confirmar, y ese es el juicio que falta |

Con las cédulas cargadas, las dos puertas cotejan igual. Esto es lo que permite construir y usar la etapa 1
**antes** de tener las cédulas, sin apagar el guardarraíl de la etapa 2.

### La misma factura en dos empresas = pago doble

El proveedor manda la factura a los dos buzones (o Contabilidad la reenvía). Los dos scripts hacen POST.
Se crean **dos** `documento_cxp` con la misma clave de 50 dígitos, uno por empresa; cada uno recibe su
huella `CXP-…` y entra en el archivo de pago de **su** banco. El proveedor cobra dos veces el mismo
comprobante, desde dos sociedades. Ninguna consulta del sistema cruza empresas para verlo.

Antes de crear el documento, la ingesta pregunta si esa clave ya existe **en cualquier empresa del grupo**
y, si existe en otra, parquea con motivo **«esta clave ya entró en otra empresa del grupo»** — sin nombrar
cuál, para no filtrar información entre empresas por el texto del error.

### Dónde vive una recepción cuyo Receptor no es la empresa del token

Con el contenido guardado bajo el `empresa_id` del token, el guardarraíl que evita crear el documento
equivocado crearía en su lugar **un repositorio de documentos de la otra empresa**: el personal de
Coopeprofa abriría la cola de errores y descargaría el XML y el PDF completos de una factura de Valle de
Paz —razón social, proveedor, líneas de detalle, montos—. Hoy eso es imposible en todo el sistema.

**Decisión:** cuando el `Receptor` no corresponde a la empresa del token, la recepción se guarda
**sin contenido**: solo clave, cédula del receptor, tipo de documento, `message_id` y motivo. El XML y el
PDF **no se persisten**. El correo sigue en Gmail, que es donde tiene que estar, y quien corresponda lo
reenvía al buzón correcto.

---

## 8. Que no se pierda ni se apague en silencio

Este capítulo existe porque el componente del que depende que no se pierda una factura —el Apps Script—
**no está versionado, no tiene tests y ningún script de despliegue lo verifica.**

### 8.1 · La etiqueta se pone DESPUÉS del 2xx, nunca antes

Hoy el script etiqueta `Procesado-XML` y después hace el POST. Si el POST falla —502 de Caddy durante
`actualizar.sh`, timeout, reinicio del binario— la siguiente corrida ya no encuentra el hilo, porque busca
`-label:Procesado-XML`. **La factura no está en CxP, no está en la cola de errores, y nadie se enteró:** la
cola solo cubre lo que llegó. Y la ventana no es teórica: el binario aplica migraciones y `EnsureDefaults`
*antes* de escuchar, así que en cada actualización está sordo ese tramo completo.

Tres reglas, y una prueba:

1. El hilo se etiqueta **solo tras un 2xx**. Cualquier 5xx, timeout o excepción de red deja el hilo sin
   etiqueta y la corrida siguiente lo vuelve a tomar. Es seguro: el `UNIQUE (empresa_id, clave)` ya impide
   el duplicado.
2. **«Ya existe» tiene que ser 2xx**, con `repetido: true` y el documento existente en el cuerpo — no un
   409. Con 409 el hilo nunca drena y la cola de errores se llena en el **camino feliz**.
3. El script manda el `Message-ID` y el ERP permite consultar por él, para poder **conciliar la recepción**:
   hilos etiquetados en Gmail contra recepciones en el ERP. La diferencia es la lista de facturas perdidas.

> **Prueba de aceptación:** apagar el backend (`docker compose stop backend`), correr el script con 5
> correos nuevos, y verificar que quedan **cero** hilos etiquetados `Procesado-XML` y que los 5 entran en
> la corrida siguiente.

### 8.2 · El latido: distinguir «no hubo facturas» de «el script está muerto»

Si Google desactiva el trigger, o alguien revoca el token, o el buzón cambia de dueño, el ERP no recibe
nada — **y en pantalla eso se ve igual que un día tranquilo.** Tres semanas después aparecen 300 facturas
sin registrar, varias ya vencidas.

Se copia el patrón que ya funciona con el BCCR (`bccr_sync_log` + «última sincronización» en pantalla):

- `cxp_fuente_recepcion.ultimo_contacto_en` se actualiza en **cada** POST, **incluso cuando la corrida no
  trajo facturas**. El script manda un latido aunque la búsqueda salga vacía. Ese es el dato que separa
  «no hubo facturas» de «esto está muerto».
- Bitácora por corrida (fuente, hilos vistos, aceptadas, parqueadas, motivo) para mostrar «última
  recepción: hace X».
- **El conteo de la cola de errores entra como una fase más del `ResumenBandeja` que ya existe**, así
  aparece en la Bandeja que Contabilidad abre todos los días. Una pantalla que hay que acordarse de abrir
  no es un aviso.

> El aviso por correo **no sirve todavía**: en producción `SMTP_ADDR` se limpia a propósito y el mailer
> devuelve «correo no configurado». Una alerta por correo hoy sería un no-op silencioso.

### 8.3 · `healthz` miente

`func health(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) }` — una línea, y **no toca la base**.
El prechequeo del script da verde mientras todo POST muere con 500. El endpoint tiene que hacer un `ping`
a Postgres para que «arriba» signifique «puede recibir».

### 8.4 · El disco, con números

| Dato | Medido |
|---|---|
| Base hoy | **257 MB** |
| Volumen de facturas | **~800/mes** (719 · 902 · 865 · 710 en jul/jun/may/abr de 2026) |
| Peso estimado por factura | XML ~10 KB + PDF ~150 KB ≈ **160 KB** |
| Crecimiento por guardar XML+PDF | **~128 MB/mes ≈ 1,5 GB/año** |

El costo real no es la base: es que **cada respaldo nocturno se lleva todo**, y `respaldar.sh` conserva
`DIAS_A_GUARDAR` copias. Con 14 días retenidos, el directorio de respaldos crece ~14× lo que crezca la
base (menos por compresión). Es sostenible un par de años en un VPS chico, pero hay que **decidir la
retención del PDF ahora**, no cuando el disco se llene.

---

## 9. Decisiones que faltan **[PENDIENTE]**

Ninguna de estas se puede deducir del código, y la regla del proyecto es explícita: no se inventan reglas
de negocio.

| # | Decisión | Por qué bloquea | Default conservador si no hay respuesta |
|---|---|---|---|
| P1 | **Qué cédulas jurídicas responde cada empresa.** Es una **lista**, no un dato por empresa: «Valle de Paz» agrupa 6 titulares (Jardines, Colinas, Religiosa, Privado de Cartago, COPENAE) según CLAUDE.md. Y hoy **no hay ningún camino de escritura a `empresa`** —el único UPDATE del backend es `tolerancia_traslado`, y las empresas se crean solo en el seed—, así que o la 0081 trae las cédulas en el SQL, o la pantalla de fuentes las administra, o la columna queda vacía para siempre | Sin ellas el cotejo del §7 no puede funcionar. Peor: el reflejo natural («si la empresa no tiene cédula, no cotejo») deja el guardarraíl **apagado desde el primer día**, y el destino de cada factura lo decidiría solo el token pegado en el script | Ninguno: es un dato, no una elección. El endpoint es **fail-closed**: sin cédulas configuradas, rechaza toda recepción con motivo explícito |
| P2 | **Un XML real de cada versión** (4.3 y 4.4), firmado, de un proveedor de verdad | Sin muestra, los nombres de los elementos se estarían *inventando* — y por la trampa del cero silencioso el resultado serían facturas en cero que pasan los tests | Ninguno |
| P3 | **Qué tipos de comprobante se vuelven cuenta por pagar.** `FacturaElectronica` sí. ¿`TiqueteElectronico`? ¿`FacturaElectronicaCompra`? ¿`FacturaElectronicaExportacion`? ¿`NotaDebitoElectronica`? | Define si entra deuda de más o se rechaza gasto real | Solo `FacturaElectronica`; el resto **DESCARTADA** conservando el XML |
| P4 | **De dónde sale el vencimiento.** El XML **no trae fecha de vencimiento**: trae `CondicionVenta` (código) y `PlazoCredito`. ¿Vencimiento = emisión + plazo del XML, o manda el `plazo_credito_dias` que el ERP ya aprendió del proveedor? | Los dos criterios existen hoy y dan resultados distintos | Emisión + `PlazoCredito` del XML (es lo que declaró el emisor) |
| P5 | **Con qué identidad corre la ingesta.** `documento_cxp.creado_por` entra como `$14::uuid` **sin NULLIF**: un usuario vacío hace fallar el INSERT. No existe usuario de sistema | Sin esto no se puede crear ni un documento desde la máquina | Un usuario técnico por integración, con su fila en `usuario` y sin contraseña utilizable |
| P6 | **Qué hacer cuando la aritmética no cuadra o falta un elemento**: ¿rechazar la fila con detalle, o crearla marcada para revisión? | Define si 4.500 facturas entran o se trancan | Rechazar (PARQUEADA) con el campo faltante en el motivo |
| P7 | **Mapeo del tipo de identificación**: el XML trae `Emisor>Identificacion>Tipo` como código (01/02/03/04) y el ERP acepta `FISICA / JURIDICA / DIMEX / NITE`. Hoy se **adivina por longitud** (9=física, 10=jurídica) | Es una regla fiscal | Seguir adivinando por longitud, como hoy |
| P8 | **Duplicado con el adjunto modificado**: el proveedor corrige el monto y reenvía con la misma clave. Hoy se rechaza con 409 y queda el monto viejo | `documento_cxp` es tabla financiera; el UPDATE fuera del flujo no corresponde | Registrar como **hallazgo** en la bandeja, nunca actualizar en silencio |
| P9 | **Desde qué fecha arranca** el script a hacer POST (los correos históricos) | Define el volumen del primer día | La fecha de puesta en marcha; el histórico ya está cargado por Excel |
| P10 | **La factura de contado que ya se pagó.** Se compra en la ferretería, se paga con caja chica, el custodio registra el vale, y la misma factura llega por correo. La ingesta la vuelve CxP a nombre del proveedor **y** la reposición del fondo crea un REINTEGRO al custodio por el mismo monto: **el gasto se paga dos veces**. Nada puede detectarlo, porque el vale de caja chica no tiene campo `clave` con el que cruzarlo | Es dinero que sale dos veces por la puerta del banco | Con `CondicionVenta` = contado, crear el documento **bloqueado para pago** con motivo «contado: confirmar si ya se pagó». Requiere código nuevo: hoy `bloqueado_para_pago` **solo se escribe en el INSERT** y no existe ninguna acción para bloquear ni desbloquear después |
| P11 | **La nota de crédito que llega antes de que se pague la factura que corrige.** Con la NC parqueada (que es lo aprobado), la factura sigue su camino y el archivo del banco lleva el monto completo | Se pagan de más los ₡ de la NC | Bloquear para pago la factura referenciada y avisar, aunque la NC todavía no se aplique. Mismo código nuevo que P10 |
| P12 | **El nombre del proveedor.** La cascada de 6 niveles del script puede terminar escribiendo el **dominio del correo** como razón social — y ese nombre viaja al archivo del banco | Un nombre inventado en una transferencia SINPE | Manda el catálogo del ERP; el nombre del XML solo se usa al **crear** un proveedor que no existe, nunca para pisar uno existente |
| P13 | **Retención del PDF.** ~1,5 GB/año en la base, multiplicado por los días de respaldo retenidos (§8.4) | El disco del VPS | Guardar XML+PDF sin poda por ahora, y revisar a los 12 meses |

**Diferidas por el usuario:** origen de la retención, soporte de EUR, notas de crédito (propuesta entregada
el 2026-09-08, esperando aprobación).

### Lo que decidí yo, para no trancar la construcción

| Tema | Decisión adoptada | Por qué es segura |
|---|---|---|
| Unidad de idempotencia | **Una fila por adjunto**, llave = `message_id` + clave normalizada (o hash del adjunto si el XML no trae clave) | Cubre el reenvío que no aporta nada *y* el correo que trae dos facturas. Con llave = solo `message_id`, un reenvío con un adjunto más perdería la factura nueva en silencio |
| Respuesta a lo repetido | **200 con `repetido: true`** y el documento existente, no 409 | Con 409 el hilo de Gmail nunca drena (§8.1) |
| Normalización de la clave | `TrimSpace` + exigir 50 dígitos, y rechazar si no | El XML viene indentado y `encoding/xml` no recorta: la clave llegaría con espacios, crearía una **segunda** fila por la misma factura, y además `fechaDeClave` fallaría por longitud y la fecha se caería al respaldo (que en UTC da el día equivocado) |
| Receptor ajeno | Se guarda **sin contenido** (§7) | Evita convertir la cola de errores en un repositorio de documentos de otra empresa |
| Autorización de la ruta de máquina | El middleware, no la matriz RBAC (D4) | `RequirePermiso` con rol vacío da 403 siempre, y con empresa vacía da **500** |
| Proveedor sin cédula en el XML | La recepción se **parquea** | Sin cédula, `resolverProveedor` da de alta **otro** proveedor en cada recepción y el UNIQUE no lo frena |

---

## 10. Orden de construcción

| Etapa | Alcance | Depende de |
|---|---|---|
| **1** | `parsearFacturasXML` + carga manual de XML/zip en la pantalla *Importar* + `io.LimitReader` + normalización de la clave en el borde | P3, P4 |

> **La etapa 1 se puede construir sin la muestra de XML (P2), y eso es mérito de D6.** Los nombres de los
> elementos del lado del dinero están **probados en producción**: el Apps Script lleva miles de facturas
> leyendo `Clave`, `NumeroConsecutivo`, `FechaEmision`, `CondicionVenta`, `PlazoCredito`,
> `Emisor>Identificacion>Numero`, `ResumenFactura`/`ResumenComprobante`,
> `CodigoTipoMoneda>CodigoMoneda`, `TipoCambio`, `TotalVenta`, `TotalDescuentos`, `TotalImpuesto` y
> `TotalComprobante`. El único nodo sin evidencia es `Receptor` —nadie lo tocó nunca— y si estuviera mal,
> la verificación aritmética y el cotejo de tres ramas lo convierten en una recepción **PARQUEADA y
> visible**, no en una factura silenciosamente equivocada. La muestra real pasa de ser *prerrequisito* a
> ser el **paso de verificación**.
| **2** | Migración 0081 · `cedula_juridica` + pantalla de fuentes de recepción · token de máquina · `POST /v1/cxp/recepcion` · bandeja de recepción con la cola de errores | P1, P5 |
| **3** | Apps Script reescrito para POST (XML + PDF + metadatos del correo), con reintento y etiquetado por resultado | etapa 2 en producción |
| **4** | Notas de crédito | aprobación de la propuesta |
| **5** | Factun por API | ~6 meses |

La etapa 1 sola ya permite dejar de usar Google Sheets, y hace que el parser quede probado contra facturas
reales antes de que la automatización se construya encima.

---

## 11. Guardarraíles que aplican (del proyecto)

- Aislamiento por empresa: `empresa_id` **siempre** del contexto, jamás del body/query/header.
- Dinero: `decimal`, jamás `float64`. `numeric` en DB → `decimal` en Go → `string` en JSON.
- Toda fecha normalizada a ISO **antes** de tocar la base (`DateStyle = ISO, MDY`).
- Tablas financieras: soft-delete + evento en `auditoria_evento`, que es append-only a nivel de motor
  (reglas de Postgres que hacen `DO INSTEAD NOTHING` en UPDATE y DELETE). Un duplicado creado por error
  **no** se limpia con un DELETE: se anula por el flujo.
- Autorización por permiso vía la matriz RBAC. Se reusa `cxp.importar` para la puerta manual; la puerta de
  máquina la custodia su middleware (D4). Si algún día se agrega un permiso nuevo, va en
  `rbac/catalogo.go` **y** en `MatrizDefault`, no solo en el router.
- Alcance vacío **cierra**, no abre.
- Cada endpoint nuevo va con test.
