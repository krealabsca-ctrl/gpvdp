-- RRHH / Nómina (Fase 3) — Etapa 2: corrida quincenal (maqueta aprobada 2026-07-22).
-- Dos corridas por mes: ADELANTO (día 15, % del salario base, sin deducciones) y
-- LIQUIDACION (día 30: mes completo con CCSS/renta/deducciones y descuento del adelanto).
-- La corrida congela un snapshot de parámetros y de la ficha (auditable); jamás se borra
-- físicamente: ANULADA es estado terminal.

-- Créditos fiscales por familia (Renta 45333-H): la ficha gana hijos y cónyuge.
ALTER TABLE empleado ADD COLUMN hijos int NOT NULL DEFAULT 0 CHECK (hijos >= 0 AND hijos <= 20);
ALTER TABLE empleado ADD COLUMN conyuge boolean NOT NULL DEFAULT false;

-- Provisiones informativas de la corrida (maqueta: aguinaldo 8.33, vacaciones 4.16, cesantía 1.50).
ALTER TABLE nomina_parametros ADD COLUMN aguinaldo_pct numeric(6, 3) NOT NULL DEFAULT 8.33;
ALTER TABLE nomina_parametros ADD COLUMN vacaciones_pct numeric(6, 3) NOT NULL DEFAULT 4.16;
ALTER TABLE nomina_parametros ADD COLUMN cesantia_pct numeric(6, 3) NOT NULL DEFAULT 1.50;

CREATE TABLE corrida_nomina (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id    uuid NOT NULL REFERENCES empresa(id),
    anio          int NOT NULL CHECK (anio >= 2024),
    mes           int NOT NULL CHECK (mes >= 1 AND mes <= 12),
    tipo          text NOT NULL CHECK (tipo IN ('ADELANTO', 'LIQUIDACION')),
    estado        text NOT NULL DEFAULT 'BORRADOR' CHECK (estado IN ('BORRADOR', 'APROBADA', 'PAGADA', 'ANULADA')),
    fecha_pago    date NOT NULL,
    parametros    jsonb NOT NULL,                  -- snapshot de nomina_parametros al calcular
    total_bruto        numeric(14, 2) NOT NULL DEFAULT 0,
    total_ccss_obrero  numeric(14, 2) NOT NULL DEFAULT 0,
    total_renta        numeric(14, 2) NOT NULL DEFAULT 0,
    total_deducciones  numeric(14, 2) NOT NULL DEFAULT 0,
    total_adelanto     numeric(14, 2) NOT NULL DEFAULT 0,
    total_neto         numeric(14, 2) NOT NULL DEFAULT 0,
    total_patronal     numeric(14, 2) NOT NULL DEFAULT 0,
    total_provisiones  numeric(14, 2) NOT NULL DEFAULT 0,
    creado_por    uuid,
    aprobado_por  uuid,
    pagado_por    uuid,
    creado_en     timestamptz NOT NULL DEFAULT now(),
    aprobado_en   timestamptz,
    pagado_en     timestamptz
);
-- Una corrida viva (no anulada) por empresa+mes+tipo; una anulada se puede rehacer.
CREATE UNIQUE INDEX uniq_corrida_viva ON corrida_nomina (empresa_id, anio, mes, tipo) WHERE estado <> 'ANULADA';

-- Colilla por empleado: snapshot de la ficha + resultados + detalle línea a línea (jsonb).
-- Se regenera al recalcular SOLO en BORRADOR (copia de trabajo); tras aprobar es inmutable.
CREATE TABLE corrida_linea (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id      uuid NOT NULL REFERENCES empresa(id),
    corrida_id      uuid NOT NULL REFERENCES corrida_nomina(id) ON DELETE CASCADE,
    empleado_id     uuid NOT NULL REFERENCES empleado(id),
    nombre          text NOT NULL,
    identificacion  text NOT NULL,
    iban            text,
    departamento    text,
    puesto          text,
    salario_base    numeric(14, 2) NOT NULL,
    hijos           int NOT NULL DEFAULT 0,
    conyuge         boolean NOT NULL DEFAULT false,
    bruto           numeric(14, 2) NOT NULL DEFAULT 0,
    base_ccss       numeric(14, 2) NOT NULL DEFAULT 0,
    base_renta      numeric(14, 2) NOT NULL DEFAULT 0,
    ccss_obrero     numeric(14, 2) NOT NULL DEFAULT 0,
    renta           numeric(14, 2) NOT NULL DEFAULT 0,
    deducciones     numeric(14, 2) NOT NULL DEFAULT 0,
    adelanto        numeric(14, 2) NOT NULL DEFAULT 0,
    neto            numeric(14, 2) NOT NULL DEFAULT 0,
    patronal        numeric(14, 2) NOT NULL DEFAULT 0,
    prov_aguinaldo  numeric(14, 2) NOT NULL DEFAULT 0,
    prov_vacaciones numeric(14, 2) NOT NULL DEFAULT 0,
    prov_cesantia   numeric(14, 2) NOT NULL DEFAULT 0,
    detalle         jsonb NOT NULL DEFAULT '[]',
    UNIQUE (corrida_id, empleado_id)
);
CREATE INDEX idx_corrida_linea ON corrida_linea (corrida_id);

-- Novedades del mes (comisiones, extras, bonos, viáticos…): solo en LIQUIDACION y BORRADOR.
CREATE TABLE corrida_novedad (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id  uuid NOT NULL REFERENCES empresa(id),
    corrida_id  uuid NOT NULL REFERENCES corrida_nomina(id) ON DELETE CASCADE,
    empleado_id uuid NOT NULL REFERENCES empleado(id),
    concepto_id uuid NOT NULL REFERENCES concepto_nomina(id),
    monto       numeric(14, 2) NOT NULL CHECK (monto > 0),
    UNIQUE (corrida_id, empleado_id, concepto_id)
);
CREATE INDEX idx_corrida_novedad ON corrida_novedad (corrida_id);
