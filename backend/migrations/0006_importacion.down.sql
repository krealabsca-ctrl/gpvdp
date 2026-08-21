ALTER TABLE cuenta_bancaria DROP CONSTRAINT IF EXISTS uq_cuenta_empresa_alias;
ALTER TABLE banco           DROP CONSTRAINT IF EXISTS uq_banco_empresa_nombre;
ALTER TABLE importacion DROP COLUMN IF EXISTS banco;
ALTER TABLE importacion DROP COLUMN IF EXISTS archivo;
