-- 0114 · Política de caja: lo que convierte una decisión de tesorería en un número proyectable.
--
-- ── POR QUÉ ESTA TABLA EXISTE ───────────────────────────────────────────────────────────────
--
-- El flujo de caja a 13 semanas necesita saber CUÁNDO va a salir la plata, y para esta empresa
-- la fecha de vencimiento no lo dice: 4.465 de 4.469 facturas abiertas ya vencieron. Proyectar
-- por vencimiento pondría ₡936 millones en la semana 1 y cero en las otras doce —un gráfico que
-- nadie volvería a abrir—.
--
-- Lo que sí lo dice es la POLÍTICA: cuánto se destina por semana a desagotar los atrasos. Eso no
-- lo puede deducir el sistema de ningún dato; es una decisión del Director Financiero, y por eso
-- se declara acá y queda con fecha y autor.

CREATE TABLE politica_caja (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id  uuid NOT NULL REFERENCES empresa(id),

    -- MONTO_FIJO: un tope en colones por semana.
    -- PCT_INGRESOS: un % de lo que se espera cobrar esa semana, que es como se maneja un
    -- desagote cuando la entrada es irregular —se paga con lo que entra, no con lo que se desea—.
    modo text NOT NULL CHECK (modo IN ('MONTO_FIJO', 'PCT_INGRESOS')),

    -- Tope semanal para pagar cartera ATRASADA. En MONTO_FIJO es el monto; en PCT_INGRESOS se
    -- ignora y manda tope_pct.
    tope_semanal numeric(16,2) NOT NULL DEFAULT 0 CHECK (tope_semanal >= 0),
    tope_pct     numeric(5,2)  NOT NULL DEFAULT 0 CHECK (tope_pct >= 0 AND tope_pct <= 100),

    -- MínimoOperativo: el saldo por debajo del cual la empresa no puede operar. No es cero: una
    -- cuenta en cero ya está en problemas. La pantalla marca la primera semana que lo cruza, y
    -- ese es el dato que se mira.
    minimo_operativo numeric(16,2) NOT NULL DEFAULT 0 CHECK (minimo_operativo >= 0),

    -- La moneda del tope. Colones y dólares se proyectan por separado —un déficit de dólares no
    -- se tapa con un superávit de colones sin ir a comprar— así que cada uno lleva su política.
    moneda text NOT NULL DEFAULT 'CRC' CHECK (moneda IN ('CRC', 'USD')),

    nota       text NOT NULL DEFAULT '',
    creado_por uuid REFERENCES usuario(id),
    creado_en  timestamptz NOT NULL DEFAULT now(),

    -- Una política vigente por empresa y moneda. Cambiarla es reemplazarla, y el histórico no
    -- hace falta: lo que se audita es el pago, no la intención.
    UNIQUE (empresa_id, moneda)
);

CREATE INDEX ix_politica_caja_empresa ON politica_caja (empresa_id);

COMMENT ON TABLE politica_caja IS
    'Cuánto se destina por semana a pagar cartera atrasada, y cuál es el saldo mínimo operativo. Alimenta el flujo de caja a 13 semanas.';

-- ── El permiso ──────────────────────────────────────────────────────────────────────────────
--
-- Declarar cuánto se paga por semana ES la decisión de tesorería. Verlo proyectado es otra cosa
-- y se cubre con `bancos.ver_proyecciones`, que ya existe: un gerente mira la curva sin poder
-- mover la política que la produce.
INSERT INTO permiso (codigo, modulo, nombre, descripcion) VALUES
    ('bancos.politica_caja',
     'Bancos',
     'Definir la política de caja',
     'Declarar cuánto se destina por semana a pagar cartera atrasada y cuál es el saldo mínimo operativo. Es lo que determina la curva del flujo a 13 semanas.')
ON CONFLICT (codigo) DO NOTHING;

-- Lo recibe quien ya decide sobre el dinero que sale: el mismo que programa pagos.
INSERT INTO rol_permiso (empresa_id, rol_id, permiso_id)
SELECT DISTINCT rp.empresa_id, rp.rol_id, p.id
FROM rol_permiso rp
JOIN permiso base ON base.id = rp.permiso_id AND base.codigo = 'bancos.ajustes'
CROSS JOIN permiso p
WHERE p.codigo = 'bancos.politica_caja'
ON CONFLICT DO NOTHING;
