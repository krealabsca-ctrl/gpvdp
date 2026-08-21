-- Fase D: tolerancia de traslado CONFIGURABLE por empresa (antes 1% hardcoded en
-- dos lugares: constante Go + literal SQL). RN-20 / decisión DF: PORCENTAJE, default 1%.
-- Es una proporción del monto mayor de las dos patas (0.01 = 1%).
ALTER TABLE empresa ADD COLUMN tolerancia_traslado numeric(6,4) NOT NULL DEFAULT 0.01;
