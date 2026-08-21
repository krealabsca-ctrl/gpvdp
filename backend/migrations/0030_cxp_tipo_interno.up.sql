-- CxP: admite el tipo INTERNO (documento interno sin factura electrónica: liquidaciones,
-- arreglos de pago, negociaciones internas). Amplía el CHECK de tipo de documento_cxp.
ALTER TABLE documento_cxp DROP CONSTRAINT documento_cxp_tipo_check;
ALTER TABLE documento_cxp ADD CONSTRAINT documento_cxp_tipo_check
    CHECK (tipo IN ('CXP', 'ANTICIPO', 'VIATICOS', 'REINTEGRO', 'INTERNO'));
