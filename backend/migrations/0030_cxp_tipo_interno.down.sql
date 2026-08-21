ALTER TABLE documento_cxp DROP CONSTRAINT documento_cxp_tipo_check;
ALTER TABLE documento_cxp ADD CONSTRAINT documento_cxp_tipo_check
    CHECK (tipo IN ('CXP', 'ANTICIPO', 'VIATICOS', 'REINTEGRO'));
