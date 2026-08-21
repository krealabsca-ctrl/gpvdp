-- ============================================================================
-- CxC · NOTAS DE CRÉDITO: bajar una deuda sin que entre plata
-- ----------------------------------------------------------------------------
-- Lo que faltaba: condonar un saldo, corregir un cargo mal generado o aplicar un
-- descuento pactado. Sin esto, la única salida era editar la base a mano.
--
-- Decisión del usuario (2026-08-05): «las autoriza el supervisor de piso, SIN
-- TOPE». Sin tope, el control no puede ser un límite de monto: tiene que ser la
-- trazabilidad. De ahí las tres reglas de este diseño:
--
--   1. MOTIVO OBLIGATORIO y con contenido (lo exige el servicio, no solo la
--      columna): una nota sin motivo es plata que se fue sin explicación.
--   2. La nota NO se borra: se ANULA, y la anulación devuelve los cargos a su
--      saldo con su antigüedad original — exactamente como la reversa de un cobro.
--   3. Consecutivo propio (NC-000001) para poder decir «se condonó con la NC-42»,
--      y auditoría de quién la emitió y quién la anuló.
--
-- La nota se aplica IGUAL que un cobro: consume `cargo_cxc.monto_aplicado`. Esa
-- decisión es la que hace que todo el resto del módulo siga funcionando sin
-- cambios — el saldo, los días de mora, el tramo, el valor esperado y el aging se
-- derivan de (monto − aplicado) y ya toman en cuenta la nota. Y como los cobros
-- viven en otra tabla, las métricas de RECAUDO no cuentan las notas como dinero.
-- ============================================================================

-- ── Consecutivo y rastro de anulación ───────────────────────────────────────
ALTER TABLE nota_credito_cxc
    ADD COLUMN consecutivo      bigint,
    ADD COLUMN anulada_por      uuid REFERENCES usuario(id),
    ADD COLUMN anulada_en       timestamptz,
    ADD COLUMN anulacion_motivo text NOT NULL DEFAULT '';

-- El consecutivo es por EMPRESA y sin huecos: se calcula dentro de la transacción
-- bajo un advisory lock. Las notas son excepcionales (no hay volumen que justifique
-- una secuencia), y una serie con huecos en un documento que justifica plata
-- condonada invita a preguntas que nadie va a poder contestar.
ALTER TABLE nota_credito_cxc ADD CONSTRAINT nota_credito_consecutivo_key
    UNIQUE (empresa_id, consecutivo);

-- ── La aplicación: a qué cargos fue la nota ─────────────────────────────────
-- Espeja `cobro_aplicacion` a propósito: una nota se reparte entre cargos con el
-- MISMO motor FIFO que ya está probado, y la anulación lo deshace igual.
CREATE TABLE nota_credito_aplicacion (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,
    nota_id    uuid NOT NULL REFERENCES nota_credito_cxc(id) ON DELETE CASCADE,
    cargo_id   uuid NOT NULL REFERENCES cargo_cxc(id),
    monto      numeric(16, 2) NOT NULL CHECK (monto > 0),
    -- parcial = la nota no alcanzó a cubrir el cargo completo.
    parcial    boolean NOT NULL DEFAULT false,
    creado_en  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (nota_id, cargo_id)
);
CREATE INDEX idx_nota_credito_aplic_cargo ON nota_credito_aplicacion (cargo_id);
CREATE INDEX idx_nota_credito_aplic_empresa ON nota_credito_aplicacion (empresa_id);

CREATE INDEX idx_nota_credito_cxc_empresa ON nota_credito_cxc (empresa_id, fecha DESC);

-- ── El rol que las autoriza ─────────────────────────────────────────────────
-- «Supervisor de piso» es un cargo real de la operación que el catálogo de roles
-- no tenía, y el usuario ya lo nombró dos veces como la autoridad: las notas de
-- crédito y los arreglos de pago a plazo largo. Se crea con los permisos que
-- necesita para ejercer esa autoridad — incluido ver la cartera, porque autorizar
-- a ciegas no es autorizar.
INSERT INTO rol (codigo, nombre, descripcion)
VALUES ('SUPERVISOR_PISO', 'Supervisor de Piso',
        'Autoriza notas de crédito (sin tope) y arreglos de pago a plazo largo en Cuentas por Cobrar')
ON CONFLICT (codigo) DO NOTHING;

INSERT INTO rol_permiso (empresa_id, rol_id, permiso_id)
SELECT e.id, r.id, p.id
FROM empresa e
CROSS JOIN rol r
JOIN permiso p ON p.codigo IN ('cxc.ver', 'cxc.gestionar', 'cxc.notas_credito', 'cxc.arreglos')
WHERE r.codigo = 'SUPERVISOR_PISO'
ON CONFLICT DO NOTHING;
