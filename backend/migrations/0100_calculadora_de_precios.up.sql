-- 0099 · Calculadora de Precios, etapas 1 y 2: catálogo de insumos y receta del plan.
--
-- POR QUÉ EXISTE. Medido sobre «Precios Calculadora Abril 2025», el archivo vigente:
--
--   · De 693 líneas de la tabla de costos, 675 NO tienen costo.
--   · En 665 de ellas el «margen» se calcula desde el precio y el precio desde el precio. Es
--     circular: no hay un costo en ninguna parte de la cadena.
--   · De las 18 líneas que sí tienen costo, el margen real promedia 47,6 % cuando la hoja declara
--     80 %. Ninguna llega al 80 %.
--   · La hoja con la que se cotiza tiene 230 líneas y CERO fórmulas: son valores pegados a mano,
--     desconectados del costo.
--
-- El propósito de estas tablas es volver a anclar el precio al costo, y que ese anclaje no se
-- pueda romper en silencio.
--
-- ── LAS TRES REGLAS QUE GOBIERNAN EL DISEÑO ─────────────────────────────────────────────────
--
-- 1 · EL COSTO SE DERIVA DEL HISTORIAL, NUNCA SE GUARDA EN EL INSUMO.
--     `insumo` no tiene columna `costo`. El costo vigente a una fecha sale de `insumo_costo`,
--     igual que la existencia en Inventario se deriva del libro de movimientos y el saldo diario
--     en Bancos se deriva de los movimientos. Un número mantenido a mano se desincroniza y
--     entonces hay dos verdades. Además es lo que permite contestar «¿qué costaba Oro en enero?»
--     sin abrir un archivo viejo.
--
-- 2 · LA BASE DEL IVA SE DECLARA, NO SE SUPONE.
--     El costeo actual dice «₡79.000» sin decir si es con o sin IVA, y la diferencia es 13 %. No
--     se puede deducir mirando el archivo. Como el IVA soportado SÍ es crédito fiscal (decisión
--     de Gerencia Financiera, 14-oct-2026), el costo que entra al modelo debe ser el de la base
--     SIN IVA; `base_iva` obliga a declarar en qué base viene el número capturado para poder
--     convertirlo. Un costo sin base declarada es un número que cada quien lee distinto.
--
-- 3 · VARIABLE Y FIJO SE SEPARAN DESDE EL INSUMO.
--     Sin esa separación no existe la contribución marginal, y sin contribución marginal nadie
--     puede contestar la pregunta que más plata mueve a diario: hasta dónde se puede descontar un
--     plan sin perder dinero. El piso de precio es el costo VARIABLE; lo fijo ya está pagado
--     véndase o no.

-- ── 1 · Catálogo de insumos ─────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS insumo (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id      uuid NOT NULL REFERENCES empresa(id),
    codigo          text NOT NULL,
    nombre          text NOT NULL,
    unidad          text NOT NULL DEFAULT 'UNIDAD',

    -- VARIABLE: se consume con cada servicio (cofre, flores, embalsame).
    -- FIJO: se absorbe por reparto y no cambia con una venta más (indirecto, sala propia).
    tipo_costo      text NOT NULL DEFAULT 'VARIABLE'
                    CHECK (tipo_costo IN ('VARIABLE','FIJO')),

    -- En qué base viene el costo que se captura para este insumo. Ver regla 2.
    base_iva        text NOT NULL DEFAULT 'SIN_IVA'
                    CHECK (base_iva IN ('SIN_IVA','CON_IVA')),

    -- Enlaces opcionales con lo que ya existe en el sistema.
    proveedor_id    uuid REFERENCES proveedor(id),
    cabys           text,

    notas           text,
    activo          boolean NOT NULL DEFAULT true,
    creado_en       timestamptz NOT NULL DEFAULT now(),
    actualizado_en  timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_insumo_empresa_codigo
    ON insumo (empresa_id, lower(codigo));
CREATE INDEX IF NOT EXISTS idx_insumo_empresa_activo
    ON insumo (empresa_id, activo);
CREATE INDEX IF NOT EXISTS idx_insumo_cabys
    ON insumo (empresa_id, cabys) WHERE cabys IS NOT NULL;

COMMENT ON TABLE insumo IS
  'Catálogo de lo que compone un plan. NO guarda el costo: el costo vive en insumo_costo, '
  'versionado por fecha de vigencia.';

-- ── 2 · El costo, versionado ────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS insumo_costo (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id      uuid NOT NULL REFERENCES empresa(id),
    insumo_id       uuid NOT NULL REFERENCES insumo(id) ON DELETE CASCADE,

    -- Siempre en la base SIN IVA. Quien captura declara su base en el insumo y el servicio
    -- convierte antes de escribir acá, para que todo lo guardado sea comparable.
    costo           numeric(16,2) NOT NULL CHECK (costo >= 0),
    moneda          text NOT NULL DEFAULT 'CRC' CHECK (moneda IN ('CRC','USD')),

    vigente_desde   date NOT NULL,

    -- De dónde salió: capturado a mano, o tomado de una factura real de CxP.
    origen          text NOT NULL DEFAULT 'MANUAL'
                    CHECK (origen IN ('MANUAL','CXP','IMPORTADO')),
    documento_cxp_id uuid REFERENCES documento_cxp(id),

    nota            text,
    creado_por      uuid REFERENCES usuario(id),
    creado_en       timestamptz NOT NULL DEFAULT now()
);

-- Un solo costo por insumo y fecha de vigencia: dos costos el mismo día es una ambigüedad que
-- ninguna consulta puede resolver sola.
CREATE UNIQUE INDEX IF NOT EXISTS ux_insumo_costo_vigencia
    ON insumo_costo (empresa_id, insumo_id, vigente_desde);
CREATE INDEX IF NOT EXISTS idx_insumo_costo_busqueda
    ON insumo_costo (empresa_id, insumo_id, vigente_desde DESC);

COMMENT ON TABLE insumo_costo IS
  'Historial de costos. El costo vigente a una fecha es el de mayor vigente_desde que no la '
  'supere. Guardado SIEMPRE en base sin IVA.';

-- ── 3 · Los planes ──────────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS plan_precio (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id      uuid NOT NULL REFERENCES empresa(id),
    codigo          text NOT NULL,
    nombre          text NOT NULL,
    familia         text NOT NULL DEFAULT 'GENERAL',

    -- DIRECTO: se cobra una vez (Jade, Oro, Diamante).
    -- SUSCRIPCION: cuota mensual recurrente (Gardenia, Olivo, Tulipán, Cedro).
    modalidad       text NOT NULL DEFAULT 'DIRECTO'
                    CHECK (modalidad IN ('DIRECTO','SUSCRIPCION')),

    -- El precio del plan directo, o la cuota del de suscripción. Uno de los dos, según modalidad;
    -- la regla la aplica el servicio, no un CHECK, porque un plan recién creado todavía no tiene
    -- precio y un CHECK impediría guardarlo mientras se arma su receta.
    precio_venta    numeric(16,2) CHECK (precio_venta IS NULL OR precio_venta >= 0),
    cuota_mensual   numeric(16,2) CHECK (cuota_mensual IS NULL OR cuota_mensual >= 0),
    moneda          text NOT NULL DEFAULT 'CRC' CHECK (moneda IN ('CRC','USD')),

    -- Solo para suscripción: cuántas personas cubre y cuánto dura, que son las dos variables que
    -- gobiernan el modelo actuarial de la etapa 5.
    beneficiarios   integer CHECK (beneficiarios IS NULL OR beneficiarios > 0),
    permanencia_meses integer CHECK (permanencia_meses IS NULL OR permanencia_meses > 0),

    notas           text,
    activo          boolean NOT NULL DEFAULT true,
    creado_en       timestamptz NOT NULL DEFAULT now(),
    actualizado_en  timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_plan_precio_empresa_codigo
    ON plan_precio (empresa_id, lower(codigo));
CREATE INDEX IF NOT EXISTS idx_plan_precio_empresa_activo
    ON plan_precio (empresa_id, activo);

-- ── 4 · La receta: qué lleva cada plan ──────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS receta_linea (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id      uuid NOT NULL REFERENCES empresa(id),
    plan_id         uuid NOT NULL REFERENCES plan_precio(id) ON DELETE CASCADE,
    insumo_id       uuid NOT NULL REFERENCES insumo(id),

    cantidad        numeric(16,4) NOT NULL CHECK (cantidad > 0),
    ordinal         integer NOT NULL DEFAULT 0,
    nota            text,
    creado_en       timestamptz NOT NULL DEFAULT now()
);

-- Un insumo una sola vez por plan: si lleva seis arreglos florales, eso es cantidad = 6, no seis
-- líneas. Repetirlo haría que el total dependa de cuántas veces alguien lo agregó.
CREATE UNIQUE INDEX IF NOT EXISTS ux_receta_plan_insumo
    ON receta_linea (empresa_id, plan_id, insumo_id);
CREATE INDEX IF NOT EXISTS idx_receta_plan
    ON receta_linea (empresa_id, plan_id, ordinal);

COMMENT ON TABLE receta_linea IS
  'Lista de materiales de un plan. El costo de la receta se calcula al LEER, tomando de '
  'insumo_costo el costo vigente a la fecha consultada.';

-- ── 5 · Los permisos ────────────────────────────────────────────────────────────────────────
--
-- Separados a propósito: ver el costeo es una cosa (lo necesita cualquiera que cotice) y
-- cambiarlo es otra (mueve el precio de todo lo que se vende).
INSERT INTO permiso (codigo, modulo, nombre, descripcion) VALUES
    ('precios.ver',
     'Calculadora de precios',
     'Ver el costeo y los márgenes',
     'Catálogo de insumos, receta de cada plan, costo y margen. Solo lectura.'),
    ('precios.editar',
     'Calculadora de precios',
     'Editar insumos, costos y recetas',
     'Capturar costos, armar la receta de un plan y fijar precios. Cambia el costeo de lo que se vende.')
ON CONFLICT (codigo) DO NOTHING;

-- Se siembra sobre las parejas (empresa, rol) QUE YA EXISTEN en rol_permiso, porque los roles
-- base son globales (empresa_id NULL) y rol_permiso lo exige NOT NULL. Sin esto el permiso
-- existe pero no llega a ningún rol, y la función queda apagada para todos salvo ADMIN.
--
-- Lectura: a quien ya podía ver el tablero de CxP, que es el mismo público que analiza gasto.
INSERT INTO rol_permiso (empresa_id, rol_id, permiso_id)
SELECT DISTINCT rp.empresa_id, rp.rol_id, p.id
FROM rol_permiso rp
JOIN permiso base ON base.id = rp.permiso_id AND base.codigo = 'cxp.dashboard'
CROSS JOIN permiso p
WHERE p.codigo = 'precios.ver'
ON CONFLICT DO NOTHING;

-- Edición: a quien ya podía tocar los parámetros de CxP. Es la misma clase de decisión
-- —configurar cómo se calcula el dinero— y no se le estrena la facultad a nadie.
INSERT INTO rol_permiso (empresa_id, rol_id, permiso_id)
SELECT DISTINCT rp.empresa_id, rp.rol_id, p.id
FROM rol_permiso rp
JOIN permiso base ON base.id = rp.permiso_id AND base.codigo = 'cxp.parametros'
CROSS JOIN permiso p
WHERE p.codigo = 'precios.editar'
ON CONFLICT DO NOTHING;
