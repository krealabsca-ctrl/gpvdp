-- Fase 2 (CxP): validación por departamento (control operativo de área, previo a la
-- aprobación financiera). Inserta el estado VALIDADO_DEPTO entre REVISADO y APROBADO.
--
-- Al documento se le agrega su departamento (centro de costo) y la huella de la validación:
-- quién validó, cuándo, con qué respaldo. Regla dura de segregación: validado_depto_por se
-- compara luego contra el aprobador (nadie valida y aprueba la misma factura).
-- Amplía el CHECK de estado para admitir VALIDADO_DEPTO (nuevo paso entre REVISADO y APROBADO).
ALTER TABLE documento_cxp DROP CONSTRAINT documento_cxp_estado_check;
ALTER TABLE documento_cxp ADD CONSTRAINT documento_cxp_estado_check
    CHECK (estado IN ('RECIBIDO', 'REVISADO', 'VALIDADO_DEPTO', 'APROBADO', 'PROGRAMADO', 'PAGADO', 'CONCILIADO',
                      'DENEGADO', 'ANULADO', 'LIQUIDADA', 'REBOTADA'));

ALTER TABLE documento_cxp ADD COLUMN departamento_id      uuid REFERENCES departamento(id);
ALTER TABLE documento_cxp ADD COLUMN validado_depto_por   uuid;
ALTER TABLE documento_cxp ADD COLUMN validado_depto_en    timestamptz;
ALTER TABLE documento_cxp ADD COLUMN validacion_respaldo  text;
ALTER TABLE documento_cxp ADD COLUMN validacion_nota      text;
CREATE INDEX idx_documento_departamento ON documento_cxp (departamento_id);

-- Validadores por departamento (titular / suplente). El gate real de "quién puede validar"
-- es esta asignación + el permiso cxp.validar_depto. Titular y suplentes cubren ausencias.
CREATE TABLE departamento_validador (
    departamento_id uuid NOT NULL REFERENCES departamento(id) ON DELETE CASCADE,
    usuario_id      uuid NOT NULL REFERENCES usuario(id) ON DELETE CASCADE,
    rol             text NOT NULL DEFAULT 'TITULAR' CHECK (rol IN ('TITULAR', 'SUPLENTE')),
    creado_en       timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (departamento_id, usuario_id)
);
