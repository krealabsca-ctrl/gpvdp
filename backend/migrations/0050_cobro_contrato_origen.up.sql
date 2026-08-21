-- El número de contrato TAL COMO VINO en el archivo, aunque no exista en la cartera.
--
-- Sin esto, un pago cuyo contrato no está cargado se muestra como «sin contrato» y el
-- operador pierde la única pista que tenía para identificarlo. Con 70 000 contratos y una
-- carga incremental, «el archivo dice CO83436 pero ese contrato no está» es un hallazgo
-- útil: o falta cargarlo, o el número viene mal.
ALTER TABLE cobro_cxc ADD COLUMN contrato_origen text NOT NULL DEFAULT '';
CREATE INDEX idx_cobro_cxc_contrato_origen ON cobro_cxc (empresa_id, contrato_origen)
    WHERE contrato_origen <> '' AND contrato_id IS NULL;
