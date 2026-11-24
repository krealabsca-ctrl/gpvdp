-- Aprobar el corte de pago antes de que salga al banco (13-nov-2026).
--
-- ── EL HUECO, MEDIDO ─────────────────────────────────────────────────────────────────────────
--
-- El lote tenía DOS estados, ABIERTO y CERRADO, y entre armar el listado y pagar no había nada.
-- En concreto:
--
--   1. La macro del banco se baja de un lote ABIERTO, sin que nadie lo apruebe. El archivo que
--      mueve el dinero no tenía puerta.
--   2. No existía forma de sacar una factura del corte. Si la gerencia financiera rechazaba una,
--      había que reabrir y rehacer el lote entero: el reproceso que esto viene a evitar.
--   3. El único rechazo que existía, REBOTADA, es el del BANCO. Y DENEGADO castiga la factura
--      entera, que no es lo que pasa cuando la gerencia dice «esta no, todavía».
--
-- ── EL FLUJO QUE SE AGREGA ───────────────────────────────────────────────────────────────────
--
--   ABIERTO  →  EN_APROBACION  →  APROBADO  →  CERRADO
--      ↑              │
--      └──────────────┘  (la gerencia devuelve el corte entero, con motivo)
--
-- Es el «payment proposal» de las organizaciones grandes (SAP F110, Oracle Payment Batch), con
-- sus tres reglas: la propuesta es editable, la aprobación deja autor y fecha, y el archivo
-- bancario se genera DESPUÉS de aprobar, nunca antes.
--
-- ── POR QUÉ LOS DOS ESTADOS VIEJOS SIGUEN SIENDO VÁLIDOS ─────────────────────────────────────
--
-- Hay 40 lotes ABIERTOS y varios CERRADOS. Un CHECK que solo admitiera el flujo nuevo los
-- volvería ilegales a todos y la migración fallaría. ABIERTO y CERRADO se conservan: los lotes
-- viejos quedan donde están y el flujo nuevo aplica de acá en adelante.

ALTER TABLE lote_pago DROP CONSTRAINT IF EXISTS lote_pago_estado_check;
ALTER TABLE lote_pago ADD CONSTRAINT lote_pago_estado_check
    CHECK (estado IN ('ABIERTO', 'EN_APROBACION', 'APROBADO', 'CERRADO'));

-- Quién lo mandó a aprobar, quién lo aprobó y quién lo devolvió.
--
-- Las tres parejas van separadas y ninguna se borra al avanzar: que un corte haya sido devuelto
-- una vez es parte de lo que hay que poder reconstruir seis meses después. `devolucion_motivo`
-- guarda la ÚLTIMA devolución; la historia completa está en `auditoria_evento`, igual que con
-- `reapertura_motivo` de la mig 0104.
ALTER TABLE lote_pago
    ADD COLUMN IF NOT EXISTS enviado_aprobacion_por uuid REFERENCES usuario (id),
    ADD COLUMN IF NOT EXISTS enviado_aprobacion_en timestamptz,
    ADD COLUMN IF NOT EXISTS aprobado_por uuid REFERENCES usuario (id),
    ADD COLUMN IF NOT EXISTS aprobado_en timestamptz,
    ADD COLUMN IF NOT EXISTS devuelto_por uuid REFERENCES usuario (id),
    ADD COLUMN IF NOT EXISTS devuelto_en timestamptz,
    ADD COLUMN IF NOT EXISTS devolucion_motivo text NOT NULL DEFAULT '';

-- La exclusión de una factura del corte.
--
-- EXCLUIR NO ES DENEGAR, y por eso no toca `estado`. Las facturas entran al lote ya estando en
-- PROGRAMADO —el lote solo les pone `lote_id`—, así que sacarlas es quitarles el lote: la factura
-- se queda en PROGRAMADO, conserva su aprobación y su validación de área, y entra al próximo
-- corte sin que nadie la vuelva a tramitar. Eso es exactamente lo que evita el reproceso.
--
-- `excluido_lote_numero` guarda de CUÁL corte salió. Sin él, una factura que volvió a la bandeja
-- no tiene cómo explicar por qué no se pagó el viernes pasado.
ALTER TABLE documento_cxp
    ADD COLUMN IF NOT EXISTS excluido_lote_por uuid REFERENCES usuario (id),
    ADD COLUMN IF NOT EXISTS excluido_lote_en timestamptz,
    ADD COLUMN IF NOT EXISTS excluido_lote_motivo text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS excluido_lote_numero bigint;

-- El índice sirve a la pregunta de la pantalla: «¿qué salió de un corte y sigue esperando?».
-- Parcial porque lo excluido es un puñado contra 4.542 facturas.
CREATE INDEX IF NOT EXISTS idx_docxp_excluido_lote
    ON documento_cxp (empresa_id, excluido_lote_en DESC)
    WHERE excluido_lote_en IS NOT NULL;

-- ── EL PERMISO ───────────────────────────────────────────────────────────────────────────────
--
-- Va SEPARADO de `cxp.tesoreria` a propósito: quien arma el corte no puede aprobarlo. Hoy los dos
-- actos caían bajo el mismo permiso y eso anula el control —la segregación de funciones es la
-- razón de ser de este paso, no un adorno—.
INSERT INTO permiso (codigo, modulo, nombre, descripcion, critico, orden)
VALUES ('cxp.aprobar_lote', 'cxp', 'Aprobar el corte de pago',
        'Revisar el cronograma de un corte y aprobarlo, devolverlo o sacarle facturas. Sin esta aprobación no se puede bajar el archivo del banco.',
        true, 0)
ON CONFLICT (codigo) DO NOTHING;

-- Se reparte a quien ya aprueba facturas: es la misma autoridad, un escalón más arriba.
--
-- `EnsureDefaults` siembra el catálogo en cada arranque pero NO reparte permisos nuevos a una
-- empresa que ya tiene su matriz configurada, así que sin este INSERT el permiso existiría y
-- nadie lo tendría.
INSERT INTO rol_permiso (empresa_id, rol_id, permiso_id)
SELECT DISTINCT rp.empresa_id, rp.rol_id, p.id
FROM rol_permiso rp
JOIN permiso base ON base.id = rp.permiso_id AND base.codigo = 'cxp.aprobar'
CROSS JOIN permiso p
WHERE p.codigo = 'cxp.aprobar_lote'
ON CONFLICT DO NOTHING;
