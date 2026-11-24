-- El banco REEMITE un movimiento: lo vuelve a mandar otro día, con otra descripción y el mismo
-- consecutivo. No es un duplicado —el detector de duplicados busca el mismo día con consecutivos
-- distintos, que es justo lo contrario— y no es un movimiento nuevo: es el mismo hecho económico
-- entrando por segunda vez.
--
-- NO SE SOBRESCRIBE NADA. Una línea importada del banco es evidencia externa: el día que alguien
-- ponga el estado de cuenta al lado del sistema tienen que calzar línea por línea, y un movimiento
-- editado ya no calza con ningún papel. Las dos filas se quedan con su fecha y su descripción
-- originales; lo único que se decide es CUÁL de las dos cuenta para los libros.
--
-- El que cuenta apunta al que sale (`reemplaza_a`), igual que una parte de un movimiento dividido
-- apunta a su origen (`dividido_de`, mig 0094). El que sale usa la puerta que ya existe —
-- `incluido = false` con su motivo y su autor (mig 0093)— así que los veinte lectores de `incluido`
-- lo dejan de sumar sin que haya que tocar ninguno.
--
-- POR QUÉ NO SE DETECTA SOLO: medido sobre los datos reales, la huella «mismo consecutivo, mismo
-- monto, misma cuenta, fechas distintas» da 13 casos en tres bancos, y casi todos son pagos
-- RECURRENTES legítimos —un ahorro de ₡1.000.000 que el BCR debita todas las semanas con el mismo
-- consecutivo, cuotas mensuales del BN—. Enlazarlos automáticamente fusionaría nueve débitos de un
-- millón en uno solo. Lo que separa una reemisión de una recurrencia es la CERCANÍA de las fechas
-- (las recurrentes caen a 7, 14, 28 o 30 días; la reemisión al día siguiente), y eso alcanza para
-- sugerir candidatos, nunca para decidir.

ALTER TABLE movimiento_bancario
    ADD COLUMN reemplaza_a      uuid REFERENCES movimiento_bancario(id),
    ADD COLUMN reemplazo_en     timestamptz,
    ADD COLUMN reemplazo_por    uuid REFERENCES usuario(id),
    ADD COLUMN reemplazo_motivo text;

-- Un movimiento no puede ser reemplazado por dos: si se permitiera, dos filas vigentes dirían
-- representar el mismo hecho y el monto entraría dos veces a los libros, que es exactamente el
-- problema que esto viene a resolver.
CREATE UNIQUE INDEX ux_mov_reemplaza_a ON movimiento_bancario (reemplaza_a)
    WHERE reemplaza_a IS NOT NULL;

-- Buscar «¿a este quién lo reemplazó?» desde la pantalla de fuera de los libros.
CREATE INDEX idx_mov_reemplazo_empresa ON movimiento_bancario (empresa_id, reemplazo_en DESC)
    WHERE reemplaza_a IS NOT NULL;

ALTER TABLE movimiento_bancario
    ADD CONSTRAINT ck_mov_reemplazo_no_propio
        CHECK (reemplaza_a IS NULL OR reemplaza_a <> id);

-- O no hay enlace, o está completo con su motivo. Un enlace sin motivo deja a quien cierre el mes
-- con una fila fuera de los libros y ninguna forma de saber si fue una corrección o un error: es
-- la misma regla que ya exige motivo para excluir a mano.
ALTER TABLE movimiento_bancario
    ADD CONSTRAINT ck_mov_reemplazo_completo
        CHECK (
            (reemplaza_a IS NULL AND reemplazo_en IS NULL AND reemplazo_motivo IS NULL)
            OR (reemplaza_a IS NOT NULL AND reemplazo_en IS NOT NULL
                AND length(btrim(coalesce(reemplazo_motivo, ''))) >= 10)
        );

COMMENT ON COLUMN movimiento_bancario.reemplaza_a IS
    'Movimiento que ESTE reemplaza: el mismo hecho que el banco reemitió. El apuntado queda fuera de los libros (incluido = false); este es el que cuenta.';
