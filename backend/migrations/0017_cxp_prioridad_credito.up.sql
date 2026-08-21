-- Bandeja CxP v2 (feedback de uso real):
-- 1) Prioridad interna de pago: AA = se paga sí o sí · A = debe pagarse pero puede esperar ·
--    '' = normal (menos prioritaria). Se fija al aprobar o en Por pagar.
-- 2) nota_revision: motivo/detalle al denegar, anular o liquidar (la "contrapartida" del archivo).
-- 3) Condiciones de crédito del proveedor (contado / crédito + plazo en días); si una factura
--    manual no trae vencimiento, se calcula emisión + plazo.
-- 4) proveedor_gasto: gastos frecuentes por proveedor (el 10% de proveedores usa varios).

ALTER TABLE documento_cxp
    ADD COLUMN prioridad text NOT NULL DEFAULT '' CHECK (prioridad IN ('', 'A', 'AA')),
    ADD COLUMN nota_revision text;

ALTER TABLE proveedor
    ADD COLUMN condicion_pago text NOT NULL DEFAULT 'CONTADO' CHECK (condicion_pago IN ('CONTADO', 'CREDITO')),
    ADD COLUMN plazo_credito_dias int NOT NULL DEFAULT 0;

CREATE TABLE proveedor_gasto (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id          uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,
    proveedor_id        uuid NOT NULL REFERENCES proveedor(id) ON DELETE CASCADE,
    concepto_id         uuid NOT NULL REFERENCES concepto(id),
    clasificacion_id    uuid REFERENCES clasificacion(id),
    subclasificacion_id uuid REFERENCES subclasificacion(id),
    usos                int NOT NULL DEFAULT 1,
    ultimo_uso          timestamptz NOT NULL DEFAULT now(),
    UNIQUE (proveedor_id, concepto_id, clasificacion_id, subclasificacion_id)
);
CREATE INDEX idx_provgasto_prov ON proveedor_gasto (proveedor_id, usos DESC);
