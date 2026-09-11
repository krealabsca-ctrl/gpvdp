-- La visibilidad para CxP baja al nivel de la CLASIFICACIÓN.
--
-- ── POR QUÉ ─────────────────────────────────────────────────────────────────
--
-- `visible_cxp` vivía solo en el CONCEPTO y la clasificación la heredaba. Medido el 9 de setiembre
-- de 2026 en Valle de Paz: **4 conceptos visibles exponen 124 clasificaciones** a Contabilidad. Un
-- solo interruptor abre 124 puertas, y entre ellas salieron gastos confidenciales que Contabilidad
-- no debe ver.
--
-- El nivel del concepto sigue existiendo y sigue mandando: es el corte grueso («Ingresos no es
-- gasto a pagar»). Lo que faltaba era el corte fino dentro de un concepto que SÍ es de CxP.
--
-- ── LA REGLA ────────────────────────────────────────────────────────────────
--
-- Visible para CxP = el concepto es visible **Y** la clasificación es visible.
--
-- Apagar el concepto oculta todo lo suyo, como hasta hoy. Apagar una clasificación oculta solo esa.
-- No hay override al revés a propósito: una clasificación visible bajo un concepto oculto sería un
-- rubro que aparece en el selector de CxP sin su padre, y eso se lee como un error del sistema.
--
-- DEFAULT true: el día del despliegue nada cambia. Hoy la visibilidad la decide el concepto, y con
-- todas las clasificaciones en `true` el resultado del AND es idéntico al de antes. Esconder algo
-- es una decisión que se toma después, tildando en el catálogo.

ALTER TABLE clasificacion
  ADD COLUMN IF NOT EXISTS visible_cxp boolean NOT NULL DEFAULT true;

-- El índice cubre la consulta del selector de gasto de CxP, que es la que corre en cada carga de la
-- bandeja: las clasificaciones visibles de una empresa.
CREATE INDEX IF NOT EXISTS idx_clasificacion_visible_cxp
  ON clasificacion (empresa_id) WHERE visible_cxp;
