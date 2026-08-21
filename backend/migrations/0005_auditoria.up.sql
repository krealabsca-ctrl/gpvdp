-- Auditoría inmutable (append-only) — spec §26.
-- empresa_id es NULLABLE: hay eventos de cuenta/identidad previos a seleccionar empresa (p.ej. LOGIN).

CREATE TABLE auditoria_evento (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id     uuid REFERENCES empresa(id),
    entidad        text NOT NULL,
    entidad_id     uuid,
    accion         text NOT NULL,
    valor_anterior jsonb,
    valor_nuevo    jsonb,
    usuario_id     uuid REFERENCES usuario(id),
    ts             timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_auditoria_empresa_ts ON auditoria_evento (empresa_id, ts);
CREATE INDEX idx_auditoria_entidad    ON auditoria_evento (entidad, entidad_id);

-- Refuerzo append-only a nivel de motor: UPDATE y DELETE no tienen efecto.
CREATE RULE auditoria_no_update AS ON UPDATE TO auditoria_evento DO INSTEAD NOTHING;
CREATE RULE auditoria_no_delete AS ON DELETE TO auditoria_evento DO INSTEAD NOTHING;

-- TRUNCATE no dispara rules; se bloquea explícitamente con un trigger.
CREATE FUNCTION auditoria_no_truncate() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'auditoria_evento es append-only: TRUNCATE no permitido';
END;
$$;
CREATE TRIGGER auditoria_no_truncate BEFORE TRUNCATE ON auditoria_evento
    FOR EACH STATEMENT EXECUTE FUNCTION auditoria_no_truncate();
