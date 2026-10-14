DELETE FROM rol_permiso WHERE permiso_id IN (SELECT id FROM permiso WHERE codigo = 'cxp.liberar_pago');
DELETE FROM permiso WHERE codigo = 'cxp.liberar_pago';

DROP INDEX IF EXISTS idx_docxp_liberadas;

-- La huella se va con la reversa. Lo que NO se toca es `bloqueado_para_pago`: una factura que se
-- liberó y se pagó tiene que seguir liberada, o la reversa de esta migración volvería a trancar
-- facturas que ya salieron por el banco.
ALTER TABLE documento_cxp
    DROP COLUMN IF EXISTS desbloqueo_motivo,
    DROP COLUMN IF EXISTS desbloqueado_en,
    DROP COLUMN IF EXISTS desbloqueado_por;

ALTER TABLE proveedor RENAME CONSTRAINT proveedor_convenio_manda_check TO proveedor_plazo_manda_check;
ALTER TABLE proveedor RENAME COLUMN convenio_manda TO plazo_manda;
