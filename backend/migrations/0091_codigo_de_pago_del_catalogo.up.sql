-- Nomenclatura interna del pago: el código con el que la empresa reconoce a qué corresponde un
-- débito en el estado de cuenta («A8 = comisiones», «A9 = planilla»).
--
-- VIVE EN EL CATÁLOGO DE GASTO, no en una tabla aparte. La decisión del Director Financiero
-- (1-oct-2026) y la razón es de escala: la factura YA tiene su gasto asignado, así que el código
-- sale solo y nadie lo teclea. Un catálogo separado obligaría a elegirlo en cada factura, y lo que
-- no se elige queda sin código.
--
-- Las dos columnas y no una: el catálogo de Valle de Paz tiene 8 conceptos y 124 clasificaciones
-- solo bajo «Gastos». Si el código viviera únicamente en la clasificación habría que cargar 124;
-- en el concepto se cargan 8 y las 124 lo heredan. La clasificación lo lleva solo cuando necesita
-- distinguirse de su concepto, y ahí manda sobre él (lo más específico gana).
ALTER TABLE concepto      ADD COLUMN IF NOT EXISTS codigo_pago text;
ALTER TABLE clasificacion ADD COLUMN IF NOT EXISTS codigo_pago text;

-- Forma: 1 a 4 caracteres alfanuméricos en MAYÚSCULAS. Corto porque va dentro de la descripción
-- que viaja al banco, junto con la referencia y el código interno, y ese campo no es infinito.
-- El CHECK acepta NULL: el código es opcional y su ausencia nunca frena un pago.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'concepto_codigo_pago_check') THEN
        ALTER TABLE concepto ADD CONSTRAINT concepto_codigo_pago_check
            CHECK (codigo_pago IS NULL OR codigo_pago ~ '^[A-Z0-9]{1,4}$');
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'clasificacion_codigo_pago_check') THEN
        ALTER TABLE clasificacion ADD CONSTRAINT clasificacion_codigo_pago_check
            CHECK (codigo_pago IS NULL OR codigo_pago ~ '^[A-Z0-9]{1,4}$');
    END IF;
END $$;

-- Un código significa UNA cosa dentro de la empresa, y para siempre. Si «A8» deja de usarse se
-- desactiva el rubro, no se le asigna a otro: un A8 de 2025 tiene que seguir queriendo decir lo
-- mismo en 2027, o el histórico miente. Los dos índices son parciales (solo donde hay código) y
-- por empresa, porque el código es un acuerdo interno de cada una.
--
-- Son índices SEPARADOS a propósito: un mismo código puede estar en un concepto y en una de sus
-- clasificaciones —ahí la clasificación solo está confirmando lo que ya heredaba— y eso no es
-- una colisión. Lo que no puede haber es dos conceptos con el mismo código, o dos clasificaciones.
CREATE UNIQUE INDEX IF NOT EXISTS idx_concepto_codigo_pago
    ON concepto (empresa_id, codigo_pago) WHERE codigo_pago IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_clasificacion_codigo_pago
    ON clasificacion (empresa_id, codigo_pago) WHERE codigo_pago IS NOT NULL;
