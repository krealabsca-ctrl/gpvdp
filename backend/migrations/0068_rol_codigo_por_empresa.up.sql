-- El código de un rol a medida es único DENTRO de su empresa, no en toda la base.
--
-- El problema que arregla (reportado 2026-08-21, con el sistema ya en producción): el usuario creó un
-- rol «Nómina» a medida en Valle de Paz y no podía darle ese mismo rol a alguien en otra empresa. Ni
-- creándolo de nuevo: `rol_codigo_key` era UNIQUE sobre `codigo` a secas, así que «Nómina» solo podía
-- existir UNA vez en todo el sistema. El administrador de la otra empresa recibía «ya existe un rol
-- con ese nombre» por un rol que su lista no muestra —existe, pero en una empresa ajena—.
--
-- El resto del modelo ya estaba preparado para esto: `rol_permiso` lleva `empresa_id` en su clave
-- primaria, o sea que los permisos de un rol SIEMPRE fueron por empresa. Lo único que faltaba era
-- dejar que el rol exista en cada empresa.
--
-- Se usan dos índices únicos PARCIALES por la misma razón que en el presupuesto (migración 0066): en
-- PostgreSQL, `UNIQUE (empresa_id, codigo)` con `empresa_id` en NULL no impide duplicados —NULL nunca
-- es igual a NULL—, y entonces se podrían crear dos roles base con el mismo código.
ALTER TABLE rol DROP CONSTRAINT IF EXISTS rol_codigo_key;

-- Roles base (empresa_id NULL): el código sigue siendo único en todo el sistema.
CREATE UNIQUE INDEX IF NOT EXISTS uq_rol_codigo_base
    ON rol (codigo)
    WHERE empresa_id IS NULL;

-- Roles a medida: el código es único dentro de su empresa. Dos empresas pueden tener su «Nómina».
CREATE UNIQUE INDEX IF NOT EXISTS uq_rol_codigo_empresa
    ON rol (empresa_id, codigo)
    WHERE empresa_id IS NOT NULL;

COMMENT ON COLUMN rol.codigo IS
    'Único entre los roles base (empresa_id NULL) y único dentro de cada empresa para los roles a medida. Dos empresas pueden tener un rol con el mismo código, cada una con sus propios permisos en rol_permiso.';
