-- Motor que aprende (Bancos Fase A): contador de aciertos por regla.
-- Se incrementa cada vez que la regla clasifica un movimiento (importación o retro-aplicación).
ALTER TABLE regla_clasificacion ADD COLUMN aciertos int NOT NULL DEFAULT 0;
