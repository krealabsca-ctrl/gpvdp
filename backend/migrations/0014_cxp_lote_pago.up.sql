-- CxP: lote de pago (corte). Se seleccionan facturas PROGRAMADAS de un corte → forman un lote
-- con número consecutivo por empresa (el "ID de lo que se va a pagar"). De ahí sale la macro.
-- Tras subir al banco: las que rebotan pasan a REBOTADA; las demás a PAGADO.

CREATE TABLE lote_pago (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id  uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,
    numero      bigint NOT NULL,
    fecha_corte date NOT NULL,
    estado      text NOT NULL DEFAULT 'ABIERTO' CHECK (estado IN ('ABIERTO', 'CERRADO')),
    creado_por  uuid REFERENCES usuario(id),
    creado_en   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (empresa_id, numero)
);
CREATE INDEX idx_lote_empresa ON lote_pago (empresa_id, creado_en DESC);

ALTER TABLE documento_cxp ADD COLUMN lote_id uuid REFERENCES lote_pago(id);
CREATE INDEX idx_docxp_lote ON documento_cxp (lote_id);

-- Estado REBOTADA: factura que el banco rechazó dentro de un lote.
ALTER TABLE documento_cxp DROP CONSTRAINT documento_cxp_estado_check;
ALTER TABLE documento_cxp ADD CONSTRAINT documento_cxp_estado_check
    CHECK (estado IN ('RECIBIDO', 'REVISADO', 'APROBADO', 'PROGRAMADO', 'PAGADO', 'CONCILIADO',
                      'DENEGADO', 'ANULADO', 'LIQUIDADA', 'REBOTADA'));
