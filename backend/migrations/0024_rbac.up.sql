-- RBAC ([T3]): catálogo de permisos + matriz permiso × rol × empresa.
-- El catálogo (filas de permiso) y la matriz por defecto se siembran de forma
-- idempotente en el arranque (rbac.EnsureDefaults), porque en una BD nueva las
-- empresas aún no existen al correr esta migración.

CREATE TABLE permiso (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    codigo      text NOT NULL UNIQUE,     -- p.ej. bancos.congelar_tc
    modulo      text NOT NULL,            -- Bancos | Cuentas por pagar | Administración
    nombre      text NOT NULL,
    descripcion text,
    critico     boolean NOT NULL DEFAULT false,
    orden       int NOT NULL DEFAULT 0
);

-- Concesión de un permiso a un rol, POR EMPRESA (una matriz por empresa).
CREATE TABLE rol_permiso (
    empresa_id uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,
    rol_id     uuid NOT NULL REFERENCES rol(id) ON DELETE CASCADE,
    permiso_id uuid NOT NULL REFERENCES permiso(id) ON DELETE CASCADE,
    creado_en  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (empresa_id, rol_id, permiso_id)
);
CREATE INDEX idx_rolpermiso_lookup ON rol_permiso (empresa_id, rol_id);

-- Los roles a medida (además de los 6 base) se distinguen por su empresa dueña.
-- NULL = rol global (los 6 base); un uuid = rol creado para esa empresa.
ALTER TABLE rol ADD COLUMN empresa_id uuid REFERENCES empresa(id) ON DELETE CASCADE;
