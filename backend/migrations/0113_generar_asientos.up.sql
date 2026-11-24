-- 0112 · El permiso de generar asientos. FASE 3B.
--
-- ── QUÉ HABILITA Y BAJO QUÉ DECISIÓN ────────────────────────────────────────────────────────
--
-- La Fase 3B genera asientos desde el movimiento bancario, es decir CONTABILIDAD POR CAJA. La
-- decisión es del Director Financiero (28-oct-2026): «no es cuando debo sino cuánto pagué, y no
-- es cuánto me deben sino cuánto ingresó. Actualmente la lógica de la empresa es cuando se
-- materializa porque el flujo se maneja bancario».
--
-- POR CAJA NO CUMPLE NIIF, y eso está dicho en la pantalla, no escondido acá. El destino es
-- devengo: `asiento.origen_tipo` acepta CXP_FACTURA desde la 0111 y la idempotencia es por
-- documento, así que agregar el devengo no toca lo generado. Lo que NO se puede es reexpresar
-- solo lo ya registrado por caja; la fecha de corte de esa migración es la decisión D3.
--
-- ── POR QUÉ UN PERMISO PROPIO ───────────────────────────────────────────────────────────────
--
-- `conta.asientos` permite escribir UN asiento a mano. Esto permite escribir MILES de una vez,
-- y revertir un mes entero. Son facultades de distinto tamaño: quien carga un ajuste puntual no
-- es necesariamente quien cierra la contabilidad de un período.

INSERT INTO permiso (codigo, modulo, nombre, descripcion) VALUES
    ('conta.generar',
     'Contabilidad',
     'Generar asientos desde el banco',
     'Convertir los movimientos bancarios de un período en asientos contables, y revertir esa generación. Contabilidad por caja: el asiento nace cuando la plata se movió.')
ON CONFLICT (codigo) DO NOTHING;

-- Lo recibe quien ya puede escribir asientos a mano: es la misma responsabilidad, a otra escala.
INSERT INTO rol_permiso (empresa_id, rol_id, permiso_id)
SELECT DISTINCT rp.empresa_id, rp.rol_id, p.id
FROM rol_permiso rp
JOIN permiso base ON base.id = rp.permiso_id AND base.codigo = 'conta.asientos'
CROSS JOIN permiso p
WHERE p.codigo = 'conta.generar'
ON CONFLICT DO NOTHING;

-- ── El índice que hace barata la idempotencia ───────────────────────────────────────────────
--
-- Previsualizar pregunta, por CADA movimiento del período, si ya tiene asiento vivo. El índice
-- único de la 0111 cubre (empresa, origen_tipo, origen_id) pero es PARCIAL sobre «no anulado»,
-- así que sirve. Lo que falta es el camino inverso que usa la reversa: todos los asientos de un
-- origen y un período.
CREATE INDEX IF NOT EXISTS ix_asiento_origen_fecha
    ON asiento (empresa_id, origen_tipo, fecha)
    WHERE estado <> 'ANULADO';
