-- Revierte la 0106.
--
-- El orden importa: primero se van las aplicaciones y las notas, y recién entonces se puede
-- devolver el CHECK a los cinco tipos viejos. Al revés, el ALTER fallaría contra cualquier
-- documento que ya sea NOTA_CREDITO y la reversa quedaría a medias.
--
-- Al borrar las aplicaciones, el neto de las facturas que tenían nota vuelve a ser el total: se
-- les pediría pagar de nuevo lo que el proveedor ya había rebajado. Por eso esta reversa solo
-- tiene sentido con la tabla vacía, que es el caso al revertir una migración recién aplicada.
DROP TABLE IF EXISTS cxp_nota_credito_aplicacion;
DELETE FROM documento_cxp WHERE tipo = 'NOTA_CREDITO';

ALTER TABLE documento_cxp DROP CONSTRAINT IF EXISTS documento_cxp_tipo_check;
ALTER TABLE documento_cxp ADD CONSTRAINT documento_cxp_tipo_check
    CHECK (tipo IN ('CXP', 'ANTICIPO', 'VIATICOS', 'REINTEGRO', 'INTERNO'));
