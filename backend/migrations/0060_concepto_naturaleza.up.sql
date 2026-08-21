-- Naturaleza del concepto: qué cuenta como INGRESO y qué como GASTO en el dashboard.
--
-- El problema: los KPIs definían ingreso = «cualquier crédito que no sea traslado» y gasto =
-- «cualquier débito que no sea traslado». Con los datos reales de agosto 2026 en Valle de Paz eso
-- daba ₡160.032.901 de ingresos y ₡88.838.685 de gastos, cuando lo que el usuario segmentó como
-- tal era ₡159.247.757 y ₡53.546.739. Los ₡35,3 millones de diferencia en gastos venían de Ahorro,
-- Reserva, Contingencia, Préstamo, Proyecto, Memorial Pets, Jefes y lo que todavía no está
-- clasificado: nada de eso es gasto operativo, y el EBITDA salía inflado.
--
-- La corrección la fijó el negocio (2026-08-12): «los ingresos es solo lo que yo segmenté como
-- ingresos, y gastos lo que segmenté como gastos». O sea que la naturaleza es una propiedad del
-- CONCEPTO, declarada por el usuario, no algo que se deduzca del signo del movimiento.
--
--   INGRESO — entra al EBITDA por el lado de los ingresos (neto: créditos − débitos del concepto,
--             así una devolución de un depósito baja el ingreso en vez de aparecer como gasto).
--   GASTO   — entra por el lado de los gastos (neto: débitos − créditos, ídem con los reembolsos).
--   NEUTRO  — NO entra al EBITDA. Movimientos de tesorería y de patrimonio: traslados, overnight,
--             ahorro, reservas, préstamos, aportes entre empresas del grupo.
--
-- El default es NEUTRO a propósito: un concepto nuevo NO debe entrar al EBITDA hasta que alguien lo
-- declare. Lo contrario es justo lo que produjo este defecto. Para que eso no se vuelva un número
-- corto en silencio, el dashboard informa cuánto queda fuera por conceptos sin declarar.

BEGIN;

ALTER TABLE concepto
    ADD COLUMN IF NOT EXISTS naturaleza text NOT NULL DEFAULT 'NEUTRO';

ALTER TABLE concepto
    DROP CONSTRAINT IF EXISTS concepto_naturaleza_check;
ALTER TABLE concepto
    ADD CONSTRAINT concepto_naturaleza_check CHECK (naturaleza IN ('INGRESO', 'GASTO', 'NEUTRO'));

COMMENT ON COLUMN concepto.naturaleza IS
    'Qué es el concepto para el EBITDA: INGRESO, GASTO o NEUTRO (no cuenta). Lo declara el usuario en el Catálogo.';

-- Arranque: solo los dos nombres que no admiten duda. El resto queda NEUTRO y lo declara el usuario
-- desde el Catálogo — no se adivina la naturaleza de «Proyecto» o «Jefes» por el signo de sus
-- movimientos, que es el error que se está corrigiendo.
UPDATE concepto SET naturaleza = 'INGRESO' WHERE lower(nombre) = 'ingresos'  AND naturaleza = 'NEUTRO';
UPDATE concepto SET naturaleza = 'GASTO'   WHERE lower(nombre) = 'gastos'    AND naturaleza = 'NEUTRO';

-- Índice para el filtro del dashboard (agrupa por naturaleza sobre el período).
CREATE INDEX IF NOT EXISTS idx_concepto_naturaleza ON concepto (empresa_id, naturaleza);

COMMIT;
