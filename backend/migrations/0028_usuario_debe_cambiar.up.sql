-- Gestión de usuarios (Administración): contraseña temporal fijada por el admin.
-- El usuario debe cambiarla en el primer ingreso (o tras un restablecimiento).
ALTER TABLE usuario ADD COLUMN debe_cambiar_password boolean NOT NULL DEFAULT false;
