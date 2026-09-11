-- CxP · Recepción de facturas electrónicas por buzón de correo (etapa 2).
--
-- Tres piezas: (1) las cédulas jurídicas de cada empresa, que son lo único contra lo que se puede
-- cotejar el Receptor del comprobante; (2) la fuente de recepción, que ES la credencial del buzón;
-- (3) la tabla de aterrizaje, donde lo recibido se guarda ANTES de intentar interpretarlo.
--
-- Ver docs/GPVDP_Ingesta_Facturas_EspecTecnica_v1.0.md.

-- ─────────────────────────────────────────────────────────────────────────────
-- 1. Las cédulas jurídicas de la empresa
-- ─────────────────────────────────────────────────────────────────────────────
--
-- Es una LISTA y no una columna a propósito: si algún día un proveedor factura a otra razón social
-- del grupo, se agrega una fila y no hace falta migración. El UNIQUE es GLOBAL sobre la cédula
-- porque una misma cédula no puede responder a dos empresas: el cotejo quedaría ambiguo y esa
-- ambigüedad es exactamente lo que haría que una factura entre en la empresa equivocada.
CREATE TABLE empresa_cedula (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,
    cedula     text NOT NULL,
    -- La razón social exacta a la que corresponde la cédula, para que se entienda al leerlo.
    titular    text NOT NULL DEFAULT '',
    -- La cédula con la que la empresa se identifica por defecto (reportes, actas).
    principal  boolean NOT NULL DEFAULT false,
    creado_en  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (cedula),
    -- Solo dígitos: un espacio pegado o un guion se volverían "otra cédula" y el cotejo fallaría.
    CONSTRAINT empresa_cedula_formato CHECK (cedula ~ '^[0-9]{9,12}$')
);
CREATE INDEX idx_empresa_cedula_empresa ON empresa_cedula (empresa_id);

-- Las tres cédulas, declaradas por el Director Financiero el 10 de setiembre de 2026.
--
-- Van en la migración y no en una pantalla porque HOY no existe ningún camino de escritura a
-- `empresa` ni a sus datos: la única creación de empresas está en el seed y el único UPDATE del
-- backend es el de la tolerancia de traslados. Sin esto, la columna nacería vacía y el cotejo del
-- Receptor quedaría apagado desde el primer día.
--
-- El emparejamiento va por NOMBRE, igual que el seed. Si una empresa no está, su INSERT no hace
-- nada (el SELECT no devuelve filas) y la migración no falla: el guardarraíl del código es
-- fail-closed, así que una empresa sin cédula rechaza la recepción en vez de aceptarla a ciegas.
INSERT INTO empresa_cedula (empresa_id, cedula, titular, principal)
SELECT e.id, '3101318985', 'VALLE DE PAZ SERVICIOS FUNERARIOS SOCIEDAD ANONIMA', true
FROM empresa e WHERE e.nombre = 'Valle de Paz'
ON CONFLICT (cedula) DO NOTHING;

INSERT INTO empresa_cedula (empresa_id, cedula, titular, principal)
SELECT e.id, '3101794025', 'MEMORIAL PETS DE COSTA RICA SOCIEDAD ANONIMA', true
FROM empresa e WHERE e.nombre = 'Memorial Pets'
ON CONFLICT (cedula) DO NOTHING;

INSERT INTO empresa_cedula (empresa_id, cedula, titular, principal)
SELECT e.id, '3004275336', 'COOPERATIVA DE SERVICIOS MULTIPLES DE PROTECCION FAMILIAR R.L', true
FROM empresa e WHERE e.nombre = 'Coopeprofa'
ON CONFLICT (cedula) DO NOTHING;

-- ─────────────────────────────────────────────────────────────────────────────
-- 2. La fuente de recepción: un buzón, una empresa, una credencial
-- ─────────────────────────────────────────────────────────────────────────────
--
-- No hay tabla de tokens aparte: la fuente ES la credencial. Una fila = un buzón de correo = una
-- empresa = un token. Calza con la decisión de tener un correo distinto por empresa, y no agrega un
-- concepto que después haya que administrar por separado.
--
-- Se guarda SOLO el hash del token, nunca el token en claro (mismo patrón que la tabla `sesion`).
-- El UNIQUE sobre el hash no es decorativo: es el índice por el que se busca al verificar.
CREATE TABLE cxp_fuente_recepcion (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id  uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,
    nombre      text NOT NULL,
    -- El buzón que atiende esta fuente. Se compara contra el que reporta el script en cada envío:
    -- sin esa comparación el campo sería decorativo y un token pegado en el script equivocado
    -- mandaría las facturas de una empresa a la otra mientras la pantalla afirma lo contrario.
    correo      text NOT NULL,
    token_hash  text NOT NULL,
    activo      boolean NOT NULL DEFAULT true,
    -- El LATIDO. Se actualiza en cada llamada, incluso cuando la corrida no trajo facturas: es el
    -- único dato que distingue «hoy no hubo facturas» de «el script está muerto». Sin esto, que la
    -- integración se apague se ve igual que un día tranquilo.
    ultimo_contacto_en timestamptz,
    creado_por  uuid REFERENCES usuario(id),
    creado_en   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (token_hash),
    UNIQUE (empresa_id, correo)
);
CREATE INDEX idx_cxp_fuente_empresa ON cxp_fuente_recepcion (empresa_id, activo);

-- ─────────────────────────────────────────────────────────────────────────────
-- 3. La tabla de aterrizaje
-- ─────────────────────────────────────────────────────────────────────────────
--
-- Lo recibido se guarda ANTES de interpretarlo, así que nada se pierde: si el XML no se entiende,
-- si el Receptor no calza o si el tipo de comprobante no genera deuda, la fila queda con el motivo
-- escrito en palabras y se puede reintentar. Es la cola de errores de recepción.
CREATE TABLE cxp_recepcion (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,
    fuente_id  uuid REFERENCES cxp_fuente_recepcion(id) ON DELETE SET NULL,

    -- IDEMPOTENCIA. Derivada del origen, nunca aleatoria: message_id del correo + la clave del
    -- comprobante (o el hash del adjunto cuando el XML no trae clave). Una fila POR ADJUNTO, así
    -- que un reenvío que agrega una factura nueva entra, y el reenvío que no aporta nada se
    -- reconoce como repetido.
    idempotency_key text NOT NULL,

    -- El comprobante
    clave          text,
    tipo_documento text,
    version_schema text,
    receptor       text,   -- la cédula que dice el XML: es lo que se coteja contra empresa_cedula
    -- El original tal como llegó. El XML es la fuente de la verdad y hoy se tira después de usarlo.
    xml_crudo      bytea,
    pdf            bytea,
    pdf_filename   text,

    -- Trazabilidad al correo, para poder conciliar «hilos etiquetados en Gmail» contra
    -- «recepciones en el ERP»: la diferencia es la lista de facturas perdidas.
    message_id text,
    asunto     text,
    remitente  text,
    buzon      text,

    -- Resultado
    estado       text NOT NULL DEFAULT 'PENDIENTE',
    motivo       text,
    documento_id uuid REFERENCES documento_cxp(id) ON DELETE SET NULL,
    intentos     int NOT NULL DEFAULT 0,
    creado_en    timestamptz NOT NULL DEFAULT now(),
    procesado_en timestamptz,

    CONSTRAINT cxp_recepcion_estado_check CHECK (estado IN
        ('PENDIENTE','PROCESADA','DUPLICADA','PARQUEADA','DESCARTADA'))
);

-- El WHERE del índice NO es opcional: NULL nunca es igual a NULL, así que sin él el índice no
-- ataja nada cuando la llave viene vacía. Misma lección que las migraciones 0066, 0067, 0069 y 0071.
CREATE UNIQUE INDEX idx_cxp_recepcion_idem ON cxp_recepcion (empresa_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL AND idempotency_key <> '';
CREATE INDEX idx_cxp_recepcion_estado ON cxp_recepcion (empresa_id, estado, creado_en DESC);
CREATE INDEX idx_cxp_recepcion_clave ON cxp_recepcion (empresa_id, clave);
CREATE INDEX idx_cxp_recepcion_msg ON cxp_recepcion (empresa_id, message_id);

-- ─────────────────────────────────────────────────────────────────────────────
-- 4. El usuario técnico de la integración
-- ─────────────────────────────────────────────────────────────────────────────
--
-- `documento_cxp.creado_por` entra al INSERT como `$14::uuid` SIN NULLIF, así que un usuario vacío
-- hace fallar la creación; y `auditoria_evento.usuario_id` es FK a usuario, así que sin una fila
-- real el evento se pierde en silencio (Registrar es best-effort). Decisión del Director Financiero
-- (2026-09-09): un usuario técnico por integración, para que la auditoría diga que lo creó la
-- máquina y no una persona.
--
-- No puede autenticarse: `activo = false` y un `password_hash` que no es un hash válido de bcrypt,
-- así que la comparación falla siempre. Y NO se le da membresía en ninguna empresa
-- (`usuario_empresa_rol`), así que no sirve para entrar a nada: la ruta de máquina se autoriza con
-- su propio middleware, no con este usuario.
INSERT INTO usuario (nombre, email, password_hash, activo)
VALUES ('Recepción de facturas (integración)', 'recepcion-facturas@sistema.local', 'sin-acceso', false)
ON CONFLICT (email) DO NOTHING;

-- ─────────────────────────────────────────────────────────────────────────────
-- 5. Permisos nuevos, con la regla «nadie gana ni pierde»
-- ─────────────────────────────────────────────────────────────────────────────
INSERT INTO permiso (codigo, modulo, nombre, descripcion) VALUES
  ('cxp.recepcion', 'Cuentas por pagar', 'Ver la recepción de facturas',
   'Bandeja de lo que llega por correo y la cola de errores de recepción; reintentar una recepción parqueada'),
  ('cxp.fuentes', 'Cuentas por pagar', 'Configurar los buzones de recepción',
   'Da de alta el correo de cada empresa y genera su credencial: define desde qué buzón el sistema acepta facturas')
ON CONFLICT (codigo) DO NOTHING;

-- Ver la recepción va con quien ya podía meter facturas al sistema (mismo trabajo, otra puerta).
INSERT INTO rol_permiso (empresa_id, rol_id, permiso_id)
SELECT rp.empresa_id, rp.rol_id, nuevo.id
FROM rol_permiso rp
JOIN permiso viejo ON viejo.id = rp.permiso_id AND viejo.codigo = 'cxp.importar'
CROSS JOIN permiso nuevo
WHERE nuevo.codigo = 'cxp.recepcion'
ON CONFLICT DO NOTHING;

-- Configurar los buzones va con quien ya configura los parámetros de CxP: crear una fuente crea
-- una credencial y decide de qué empresa son las facturas que entran. No es una tarea de operación.
INSERT INTO rol_permiso (empresa_id, rol_id, permiso_id)
SELECT rp.empresa_id, rp.rol_id, nuevo.id
FROM rol_permiso rp
JOIN permiso viejo ON viejo.id = rp.permiso_id AND viejo.codigo = 'cxp.parametros'
CROSS JOIN permiso nuevo
WHERE nuevo.codigo = 'cxp.fuentes'
ON CONFLICT DO NOTHING;
