-- De dónde sale la fecha de vencimiento de una factura: del XML o del convenio con el proveedor.
--
-- El caso, del Director Financiero (9-oct-2026): «hay proveedores que en el XML su plazo por
-- defecto es 90 días, pero el crédito real en las facturas o por convenio es otro, y eso nos está
-- malogrando la data. Hay que validar y permitirnos configurar por proveedor si la fecha de
-- vencimiento aplica la del XML o bien la que uno configuró en cada proveedor.»
--
-- Hoy el vencimiento que viene en el comprobante SIEMPRE gana: `CrearDocumento` hace
-- `COALESCE(vencimiento_recibido, emisión + plazo_del_proveedor)`, así que el plazo configurado
-- solo se usa cuando el XML no trae ninguno. Medido en la base local: **154 facturas de 36
-- proveedores** tienen un vencimiento que no coincide con el plazo pactado con ese proveedor.
--
-- Eso no es un error del XML: el proveedor declara el plazo de su catálogo y el convenio es otra
-- cosa. Por eso no se cambia el default —el comprobante sigue mandando mientras nadie diga lo
-- contrario— y lo que se agrega es poder decir, PROVEEDOR POR PROVEEDOR, que el convenio manda.

BEGIN;

ALTER TABLE proveedor
    ADD COLUMN plazo_manda text NOT NULL DEFAULT 'XML'
        CHECK (plazo_manda IN ('XML', 'PROVEEDOR'));

COMMENT ON COLUMN proveedor.plazo_manda IS
  'De dónde sale el vencimiento de las facturas de este proveedor: XML = el plazo que declara el comprobante (default, comportamiento histórico); PROVEEDOR = emisión + plazo_credito_dias, el convenio.';

-- El default es 'XML' y no 'PROVEEDOR' a propósito: es exactamente lo que el sistema hacía antes,
-- así que esta migración no mueve ni un vencimiento de los 4.542 documentos que ya existen.
-- Cambiar el default habría recalculado en silencio las fechas de toda la cartera, que es
-- justamente el tipo de cosa que esta función viene a evitar.

-- Un índice no hace falta: se lee junto al proveedor, que ya viene por su clave primaria.

COMMIT;
