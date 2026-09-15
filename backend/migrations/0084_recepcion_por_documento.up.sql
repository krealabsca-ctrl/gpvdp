-- VER LA FACTURA DESDE LA BANDEJA (14 de setiembre de 2026).
--
-- El visor de comprobante lee el XML original que quedó guardado en `cxp_recepcion`. Para poder
-- ofrecerlo también desde la Bandeja hay que ir en sentido CONTRARIO al que existe: hoy la
-- recepción sabe qué documento creó (`cxp_recepcion.documento_id`), pero desde el documento no hay
-- forma de llegar a su recepción sin recorrer la tabla entera.
--
-- Sin este índice, el listado de la Bandeja —que trae hasta 200 filas y ya hace cinco JOIN—
-- sumaría un recorrido secuencial de `cxp_recepcion` por cada página.
--
-- Es PARCIAL a propósito: solo interesan las filas que efectivamente crearon un documento. Las
-- parqueadas y descartadas nunca se buscan por esta vía, y dejarlas fuera mantiene el índice del
-- tamaño de lo que de verdad se consulta.
CREATE INDEX IF NOT EXISTS idx_cxp_recepcion_documento
  ON cxp_recepcion (documento_id)
  WHERE documento_id IS NOT NULL;
