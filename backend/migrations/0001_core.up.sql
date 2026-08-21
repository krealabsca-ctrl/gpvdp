-- Núcleo de identidad y multi-tenant.
-- gen_random_uuid() es una función del core de PostgreSQL 13+ (no requiere extensión).

CREATE TABLE empresa (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    nombre     text NOT NULL,
    tipo_legal text,
    activo     boolean NOT NULL DEFAULT true,
    creado_en  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (nombre)
);

CREATE TABLE rol (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    codigo      text NOT NULL,
    nombre      text NOT NULL,
    descripcion text,
    creado_en   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (codigo)
);

CREATE TABLE usuario (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    nombre         text NOT NULL,
    email          text NOT NULL,
    password_hash  text NOT NULL,
    activo         boolean NOT NULL DEFAULT true,
    creado_en      timestamptz NOT NULL DEFAULT now(),
    actualizado_en timestamptz NOT NULL DEFAULT now(),
    UNIQUE (email)
);

-- Pertenencia usuario × empresa × rol. Un usuario puede operar varias empresas.
CREATE TABLE usuario_empresa_rol (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,
    usuario_id uuid NOT NULL REFERENCES usuario(id) ON DELETE CASCADE,
    rol_id     uuid NOT NULL REFERENCES rol(id),
    creado_en  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (empresa_id, usuario_id)
);
CREATE INDEX idx_uer_usuario ON usuario_empresa_rol (usuario_id);
CREATE INDEX idx_uer_empresa ON usuario_empresa_rol (empresa_id);

-- Refresh tokens revocables. Se persiste solo el hash del token, nunca el token en claro.
CREATE TABLE sesion (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    usuario_id uuid NOT NULL REFERENCES usuario(id) ON DELETE CASCADE,
    token_hash text NOT NULL,
    expira_en  timestamptz NOT NULL,
    revocado   boolean NOT NULL DEFAULT false,
    creado_en  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (token_hash)
);
CREATE INDEX idx_sesion_usuario ON sesion (usuario_id);
