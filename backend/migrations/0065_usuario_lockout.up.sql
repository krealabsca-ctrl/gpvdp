-- Bloqueo de cuenta ante fuerza bruta de login.
-- intentos_fallidos: contador de fallos CONSECUTIVOS (se pone a 0 en cada login exitoso).
-- bloqueado_hasta: si está en el futuro, el login se rechaza aunque la contraseña sea correcta.
ALTER TABLE usuario ADD COLUMN IF NOT EXISTS intentos_fallidos int NOT NULL DEFAULT 0;
ALTER TABLE usuario ADD COLUMN IF NOT EXISTS bloqueado_hasta timestamptz;
