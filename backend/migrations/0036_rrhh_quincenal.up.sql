-- RRHH / Nómina — pago QUINCENAL real y frecuencia de deducciones (maqueta aprobada
-- 2026-07-29). Decisiones del Director Financiero:
--   1) El tratamiento depende de la JORNADA de la ficha: QUINCENAL recibe dos salarios
--      reales (cada quincena con su CCSS, renta y deducciones); el resto sigue con
--      adelanto el día 15 + liquidación el 30.
--   2) Renta: la 1ª quincena retiene la mitad del impuesto mensual estimado y la 2ª
--      recalcula sobre el mes real y cobra la diferencia (los tramos son MENSUALES:
--      gravar cada mitad por separado retendría de menos).
--   3) Cada deducción tiene su frecuencia de cobro.

-- Frecuencia de cobro de cada deducción recurrente.
ALTER TABLE deduccion_empleado ADD COLUMN frecuencia text NOT NULL DEFAULT 'MENSUAL'
    CHECK (frecuencia IN ('AMBAS', 'PRIMERA', 'SEGUNDA', 'MENSUAL'));
COMMENT ON COLUMN deduccion_empleado.frecuencia IS
    'AMBAS = cada quincena · PRIMERA/SEGUNDA = solo esa quincena · MENSUAL = una vez al mes (en la 2ª)';

-- Tratamiento aplicado a la colilla (queda en el snapshot, es auditable y explica el cálculo).
ALTER TABLE corrida_linea ADD COLUMN tratamiento text NOT NULL DEFAULT 'MENSUAL'
    CHECK (tratamiento IN ('QUINCENA_1', 'QUINCENA_2', 'ADELANTO', 'MENSUAL'));
COMMENT ON COLUMN corrida_linea.tratamiento IS
    'QUINCENA_1/QUINCENA_2 = salario quincenal real · ADELANTO = anticipo sin deducciones · MENSUAL = liquidación del mes';
