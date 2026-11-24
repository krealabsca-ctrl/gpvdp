-- 0104 · El lote de pago se puede cerrar, y reabrir dejando por qué.
--
-- ── EL PROBLEMA MEDIDO (auditoría Bancos↔CxP, hallazgo 3.3) ─────────────────────────────────
--
--     39 lotes en la base. Los 39 en estado ABIERTO.
--
-- `lote_pago.estado` tiene CHECK (estado IN ('ABIERTO','CERRADO')) desde que existe la tabla, y
-- NINGÚN punto del código escribe 'CERRADO' — verificado por búsqueda en todo `internal/`. Es una
-- columna con restricción, dos valores posibles y un solo escritor que siempre escribe el mismo.
--
-- Lo que eso cuesta en la operación:
--
--   · NO HAY CORTE. Un lote cuya macro ya se subió al banco sigue aceptando facturas. Lo que se
--     pagó y lo que el lote dice contener pueden separarse sin que nada lo impida, y el reporte
--     del lote describe un conjunto que todavía puede cambiar después de emitido.
--   · NO HAY PUNTO DE CONTROL. El cierre es el momento natural para preguntar si todas las
--     facturas del lote tienen ya su débito en el banco — que es el hallazgo 3.1.
--
-- ── POR QUÉ SE PUEDE REABRIR, Y CON MOTIVO ──────────────────────────────────────────────────
--
-- Decisión del Director Financiero (20-oct-2026): cerrar BLOQUEA agregar facturas, pero se puede
-- reabrir dejando el motivo.
--
-- Un candado sin llave es un defecto, no un control: es exactamente lo que pasó con
-- `bloqueado_para_pago` —doce lectores, un solo escritor, y la factura trancada para siempre
-- hasta que la 0097 le puso la llave— y con `incluido` antes de la 0093. Un lote cerrado por
-- error sin forma de reabrirlo obligaría a crear otro lote, dejando la macro ya emitida huérfana.
--
-- El motivo es obligatorio por lo mismo que en `liberar` y en `cerrar por otro medio`: reabrir un
-- lote cerrado es afirmar que el corte que alguien dio por terminado no estaba terminado. Si en
-- seis meses el contenido de un lote no cuadra con la macro que se subió al banco, lo único que
-- permite reconstruir qué pasó es quién lo reabrió, cuándo y con qué argumento.
--
-- NO SE TOCA NINGÚN LOTE EXISTENTE. Las columnas nacen nulas y los 39 lotes siguen ABIERTO, que
-- es como el sistema los viene tratando. Cerrar es una acción que alguien decide, no un efecto de
-- esta migración.

ALTER TABLE lote_pago
    ADD COLUMN IF NOT EXISTS cerrado_por       uuid REFERENCES usuario(id),
    ADD COLUMN IF NOT EXISTS cerrado_en        timestamptz,
    ADD COLUMN IF NOT EXISTS reabierto_por     uuid REFERENCES usuario(id),
    ADD COLUMN IF NOT EXISTS reabierto_en      timestamptz,
    ADD COLUMN IF NOT EXISTS reapertura_motivo text;

COMMENT ON COLUMN lote_pago.cerrado_en IS
  'Cuándo se dio el corte. Con el lote cerrado no se le pueden agregar más facturas.';
COMMENT ON COLUMN lote_pago.reapertura_motivo IS
  'Por qué se reabrió un lote ya cerrado. Obligatorio al reabrir: es la única huella de que el '
  'corte que alguien dio por terminado se volvió a mover. Guarda la ÚLTIMA reapertura; la '
  'historia completa vive en auditoria_evento.';

-- ── El permiso ──────────────────────────────────────────────────────────────────────────────
--
-- CERRAR no lleva permiso propio: lo hace quien arma el lote y sube la macro, que ya tiene
-- `cxp.tesoreria`. Pedirle un permiso extra a la acción ordenada —la que deja el sistema en el
-- estado correcto— es la forma segura de que nadie la use.
--
-- REABRIR sí, y por la misma razón que `cxp.liberar_pago`: es levantar un control, no ejercerlo.
INSERT INTO permiso (codigo, modulo, nombre, descripcion) VALUES
    ('cxp.reabrir_lote',
     'Cuentas por pagar',
     'Reabrir un lote de pago ya cerrado',
     'Permite volver a agregar facturas a un lote que ya se había cerrado. Exige motivo y queda registrado quién lo hizo.')
ON CONFLICT (codigo) DO NOTHING;

-- Se siembra sobre las parejas (empresa, rol) QUE YA EXISTEN en `rol_permiso`, porque los roles
-- base son globales (empresa_id NULL) y `rol_permiso` lo exige NOT NULL. Lo recibe quien ya podía
-- APROBAR el pago: es la misma firma —decir que este corte se vuelve a mover— y no se le estrena
-- la facultad a nadie que no la tuviera en otra forma.
INSERT INTO rol_permiso (empresa_id, rol_id, permiso_id)
SELECT DISTINCT rp.empresa_id, rp.rol_id, p.id
FROM rol_permiso rp
JOIN permiso apr ON apr.id = rp.permiso_id AND apr.codigo = 'cxp.aprobar'
CROSS JOIN permiso p
WHERE p.codigo = 'cxp.reabrir_lote'
ON CONFLICT DO NOTHING;
