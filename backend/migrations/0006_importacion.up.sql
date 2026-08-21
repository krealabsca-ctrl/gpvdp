-- Fase 1: guardar el archivo original (RN-06) y claves de negocio para seed idempotente.

ALTER TABLE importacion ADD COLUMN archivo bytea;
ALTER TABLE importacion ADD COLUMN banco   text;

-- Claves de negocio (habilitan upsert idempotente del seed de cuentas).
ALTER TABLE banco           ADD CONSTRAINT uq_banco_empresa_nombre UNIQUE (empresa_id, nombre);
ALTER TABLE cuenta_bancaria ADD CONSTRAINT uq_cuenta_empresa_alias UNIQUE (empresa_id, alias);
