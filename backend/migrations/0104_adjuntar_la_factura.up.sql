-- 0103 · La FACTURA del proveedor también se puede adjuntar, no solo el comprobante del banco.
--
-- ── EL PROBLEMA MEDIDO (Director Financiero, 19-oct-2026) ───────────────────────────────────
--
-- «afuera en cada factura debería verse la factura original, el comprobante de pago, enviar el
-- comprobante por correo o whatsapp, y demás».
--
-- La mitad del expediente NO EXISTE. Medido al escribir esto:
--
--     facturas en el sistema ................ 4.542
--     con su XML guardado ...................     0
--     filas en cxp_recepcion (el buzón) .....     0
--
-- Las 4.542 entraron por Excel, y el único camino que conserva el documento original —el buzón de
-- correo— nunca recibió nada. Así que «ver la factura» no es un botón que falta: es un documento
-- que el sistema no tiene de dónde sacar.
--
-- Validar un pago es cruzar DOS papeles: lo que el proveedor facturó y lo que el banco debitó. Con
-- uno solo, la pantalla puede mostrar el pago pero no permite verificarlo.
--
-- ── POR QUÉ ESTA TABLA Y NO UNA NUEVA ───────────────────────────────────────────────────────
--
-- `comprobante_pago` ya es «un archivo adjunto a una factura, con su mime y quién lo subió», y
-- alrededor hay maquinaria que sirve igual para los dos documentos: adjuntar, descargar, reemplazar
-- y la carpeta masiva que reparte por el código del nombre. Una tabla gemela obligaría a duplicar
-- todo eso, y dos copias de la misma lógica divergen.
--
-- EL NOMBRE SE QUEDA. Renombrarla a algo como `documento_adjunto` sería más honesto, pero son
-- veinte referencias en código y consultas para ganar claridad en un nombre: el riesgo supera al
-- beneficio. Queda escrito acá y en el código que esta tabla guarda LOS DOS documentos.

ALTER TABLE comprobante_pago
    ADD COLUMN tipo text NOT NULL DEFAULT 'COMPROBANTE';

-- COMPROBANTE: el recibo del banco (lo que ya había).
-- FACTURA: el documento que emitió el proveedor.
ALTER TABLE comprobante_pago
    ADD CONSTRAINT comprobante_pago_tipo_check
        CHECK (tipo IN ('COMPROBANTE', 'FACTURA'));

-- La unicidad pasa a ser POR TIPO: una factura puede tener su comprobante Y su factura, pero no
-- dos comprobantes. Sin esto, adjuntar la factura borraría el recibo del banco por el UNIQUE
-- viejo, y el expediente quedaría peor que antes.
ALTER TABLE comprobante_pago
    DROP CONSTRAINT comprobante_pago_documento_id_key;

CREATE UNIQUE INDEX ux_adjunto_documento_tipo
    ON comprobante_pago (documento_id, tipo);
