-- Avisar de un movimiento que NO aparece.
--
-- ── EL HUECO ────────────────────────────────────────────────────────────────
--
-- El aviso de mala segmentación cuelga de un movimiento, así que vive en cada fila de la pantalla.
-- Si el movimiento no aparece no hay fila, y por lo tanto no hay forma de avisar — que es justo el
-- caso más común: el equipo espera un depósito y no lo ve.
--
-- Y no lo ve por cuatro razones distintas que no puede distinguir: no se depositó, no se importó el
-- archivo del banco de ese día, entró y quedó SIN CLASIFICAR, o entró y lo clasificaron en OTRA
-- partida. Las dos últimas son las que hay que corregir; las dos primeras no son un problema del
-- ERP y el aviso sería ruido.
--
-- ── QUÉ CAMBIA ──────────────────────────────────────────────────────────────
--
-- El aviso pasa a tener dos formas, y la tabla es la misma porque la cola de quien clasifica es una:
--
--   · sobre un movimiento que el equipo VE      → `movimiento_id`
--   · sobre un movimiento que ESPERA y no ve    → `fecha_esperada` + `monto_esperado`
--
-- En el segundo caso el servidor intenta ENGANCHAR el movimiento: si hay exactamente un crédito de
-- esa fecha y ese monto en la empresa, lo adjunta. Si hay varios no adivina —dos depósitos
-- idénticos el mismo día son indistinguibles— y el aviso viaja sin enganche, con la referencia que
-- escribió el equipo.

ALTER TABLE movimiento_reporte_segmentacion
  ALTER COLUMN movimiento_id DROP NOT NULL,
  ADD COLUMN IF NOT EXISTS fecha_esperada date,
  ADD COLUMN IF NOT EXISTS monto_esperado numeric(16,2),
  ADD COLUMN IF NOT EXISTS referencia text;

-- Un aviso es de una forma o de la otra, nunca de ninguna. Sin esta guarda entraría un aviso vacío
-- —sin movimiento y sin lo que se esperaba— que nadie puede investigar.
ALTER TABLE movimiento_reporte_segmentacion
  DROP CONSTRAINT IF EXISTS reporte_tiene_de_que_habla;
ALTER TABLE movimiento_reporte_segmentacion
  ADD CONSTRAINT reporte_tiene_de_que_habla CHECK (
    movimiento_id IS NOT NULL
    OR (fecha_esperada IS NOT NULL AND monto_esperado IS NOT NULL AND monto_esperado > 0)
  );

-- El índice de «un solo aviso abierto por movimiento» sigue sirviendo: en Postgres los NULL no
-- chocan entre sí, así que varios faltantes conviven sin tocarlo.
--
-- Para los faltantes hace falta su propio tope, o la misma persona levanta diez veces el mismo
-- («no me aparecen los ₡4.950 del 3») y la cola se llena de una sola duda. Se limita por PERSONA y
-- no globalmente: dos equipos distintos pueden estar esperando el mismo monto el mismo día.
CREATE UNIQUE INDEX IF NOT EXISTS uq_faltante_abierto_por_usuario
  ON movimiento_reporte_segmentacion (usuario_id, fecha_esperada, monto_esperado)
  WHERE resuelto_en IS NULL AND movimiento_id IS NULL;
