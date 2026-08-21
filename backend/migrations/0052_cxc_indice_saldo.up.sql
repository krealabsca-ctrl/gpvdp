-- ============================================================================
-- CxC · el índice que sostiene la cartera a escala real
-- ----------------------------------------------------------------------------
-- Medido con 70 000 contratos y 1 053 008 cargos (124 299 abiertos), que es el
-- volumen declarado del negocio:
--
--   sin este índice:  Bitmap Heap Scan + HashAggregate que se DERRAMA A DISCO
--                     (21 lotes, 7 MB de temporales), 18 769 bloques leídos,
--                     346 ms solo el agregado.
--   con el índice:    Index Only Scan + GroupAggregate, 1 024 bloques, 94 ms.
--
-- Por qué funciona: la consulta que alimenta la cartera y la cola agrupa los
-- cargos ABIERTOS por contrato y suma monto − aplicado. El índice que ya existía
-- (empresa_id, vence_en) sirve para filtrar por fecha, pero no puede agrupar por
-- contrato ni evitar ir al heap por el monto. Este los ordena por contrato e
-- INCLUYE las tres columnas que se suman, así que la consulta se resuelve sin
-- tocar la tabla.
--
-- Es PARCIAL: solo indexa lo que todavía se debe. Con el tiempo la mayoría de los
-- cargos quedan saldados, así que el índice crece con la mora, no con la historia
-- (8,2 MB para 124 299 cargos abiertos).
-- ============================================================================

CREATE INDEX IF NOT EXISTS idx_cargo_cxc_saldo ON cargo_cxc (empresa_id, contrato_id)
    INCLUDE (vence_en, monto, monto_aplicado)
    WHERE estado IN ('ABIERTO', 'PARCIAL');

-- Los cobros se consultan por contrato para derivar si una promesa se cumplió.
-- Sin esto, cada promesa evaluada era un escaneo de cobro_cxc.
CREATE INDEX IF NOT EXISTS idx_cobro_cxc_contrato_vivo ON cobro_cxc (contrato_id, fecha_bancaria)
    WHERE estado <> 'REVERSADO';
