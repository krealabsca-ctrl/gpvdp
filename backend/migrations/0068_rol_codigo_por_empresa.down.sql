-- Volver a la unicidad global del código exige que no haya códigos repetidos entre empresas. Si los
-- hay, esto falla a propósito: elegir cuál rol sobrevive borraría los permisos de una empresa y
-- dejaría usuarios sin rol. Hay que resolverlo a mano antes de bajar la migración.
DROP INDEX IF EXISTS uq_rol_codigo_empresa;
DROP INDEX IF EXISTS uq_rol_codigo_base;

ALTER TABLE rol ADD CONSTRAINT rol_codigo_key UNIQUE (codigo);
