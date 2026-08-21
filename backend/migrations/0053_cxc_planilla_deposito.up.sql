-- ============================================================================
-- CxC · PLANILLAS DE ASOCIACIÓN: el tercer contraste contra el depósito bancario
-- ----------------------------------------------------------------------------
-- Hasta ahora el panorama comparaba dos cosas: lo ESPERADO (los cargos que
-- vencen) contra lo REGISTRADO (los cobros del detalle que manda la asociación).
-- Faltaba la tercera: lo que de verdad ENTRÓ al banco.
--
-- Lo que mandó el negocio (confirmado por el usuario el 2026-08-05): «normalmente
-- nos envía un correo con el comprobante bancario; si ves Bancos ya hay muchos
-- movimientos de asociaciones». O sea: el monto NO se captura a mano — sale del
-- movimiento bancario que ya está importado. El correo es el aviso, no la fuente.
--
-- Y lo que decidieron los DATOS REALES (192 créditos ya clasificados como
-- «Ingresos › Asociaciones» en Valle de Paz):
--
--   · la descripción del banco casi nunca dice DE QUÉ asociación es. Hay 31
--     movimientos que solo dicen «TEF DE:ASOCIACION SOLIDARISTA» y decenas de
--     «ASOCIACION SOLIDARIS» (truncado). Solo algunos la nombran: ASEPANDUIT,
--     ASELCA, ASEGAZ, ASEKFC, ASEMAYCA, ASOET.
--     ⇒ El emparejamiento automático por nombre es IMPOSIBLE en general. Por eso
--       el operador VINCULA, y el sistema propone candidatos y explica por qué.
--   · una misma planilla llega en varias transferencias (su propio dato traía
--     «08/07/2026|11/07/2026») ⇒ la relación es de uno a MUCHOS movimientos.
-- ============================================================================

-- ── La planilla se adelgaza: los montos se DERIVAN ──────────────────────────
-- `esperado` salía de los cargos, `depositado` de los movimientos vinculados y
-- el `estado` de comparar los tres. Guardarlos era tener dos verdades y un
-- trabajo de sincronización que nadie iba a hacer: la misma disciplina que el
-- saldo del contrato y el cumplimiento de las promesas.
-- La tabla está VACÍA (nunca tuvo código detrás), así que se puede corregir sin
-- migrar ni un dato.
ALTER TABLE cxc_planilla
    DROP COLUMN esperado,
    DROP COLUMN depositado,
    DROP COLUMN fechas_bancarias,
    DROP COLUMN estado;

-- La referencia del comprobante ya no puede ser la llave: llega por correo y a
-- veces no llega. Una asociación tiene UNA planilla por período (el período
-- lleva la quincena cuando aplica: «2026-07» o «2026-07-1Q»).
ALTER TABLE cxc_planilla DROP CONSTRAINT cxc_planilla_empresa_id_asociacion_id_referencia_key;
ALTER TABLE cxc_planilla ALTER COLUMN referencia SET DEFAULT '';
ALTER TABLE cxc_planilla ADD CONSTRAINT cxc_planilla_periodo_key
    UNIQUE (empresa_id, asociacion_id, periodo);
ALTER TABLE cxc_planilla ADD CONSTRAINT cxc_planilla_periodo_no_vacio CHECK (periodo <> '');

DROP INDEX IF EXISTS idx_cxc_planilla_empresa;
CREATE INDEX idx_cxc_planilla_empresa ON cxc_planilla (empresa_id, periodo);

-- ── El vínculo con Bancos ───────────────────────────────────────────────────
-- Un movimiento bancario pertenece a lo sumo A UNA planilla: sin UNIQUE, el
-- mismo depósito podría dar por conciliadas dos asociaciones a la vez y el
-- cuadre diría que entró el doble.
--
-- Partir un movimiento entre dos planillas (una transferencia que cubre dos
-- asociaciones) NO se contempla todavía: exigiría una regla de reparto que el
-- negocio no ha definido. Si aparece, se agrega un `monto` a esta tabla.
CREATE TABLE cxc_planilla_movimiento (
    empresa_id  uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,
    planilla_id uuid NOT NULL REFERENCES cxc_planilla(id) ON DELETE CASCADE,
    movimiento_bancario_id uuid NOT NULL REFERENCES movimiento_bancario(id) ON DELETE CASCADE,
    vinculado_por uuid REFERENCES usuario(id),
    vinculado_en  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (planilla_id, movimiento_bancario_id),
    -- Un depósito, una planilla.
    UNIQUE (movimiento_bancario_id)
);
CREATE INDEX idx_planilla_mov_empresa ON cxc_planilla_movimiento (empresa_id);

-- ── Tolerancia de conciliación ──────────────────────────────────────────────
-- Arranca en CERO a propósito: no se inventa una tolerancia. Si resulta que las
-- asociaciones depositan neto de comisión bancaria, la diferencia va a aparecer
-- en pantalla con su monto exacto y ahí se decide cuánto tolerar.
INSERT INTO cxc_parametro (empresa_id, clave, valor, descripcion)
SELECT e.id, 'PLANILLA_TOLERANCIA', '0',
       'Diferencia en colones que se tolera entre lo depositado y lo registrado antes de marcar la planilla con diferencia'
FROM empresa e
ON CONFLICT DO NOTHING;
