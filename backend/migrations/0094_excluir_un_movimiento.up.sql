-- Sacar de los libros UN movimiento, no la carga entera.
--
-- El caso, del Director Financiero (9-oct-2026): el BAC cambió el formato del consecutivo y el
-- mismo depósito entró dos veces —«82007293 / DEPOSITO CAJERO AUTOMATICO CRC» y «220007293 /
-- DEP_ATM_207710067 ALA», mismo día, mismo monto, mismos cuatro dígitos finales—. «Cuando se cargó
-- ya no puedo excluir el movimiento y se malogró la data, ya no es fidedigna.»
--
-- Y tenía razón: `incluido` tiene veinte lectores y UN SOLO escritor, la reversa de una
-- importación (mig 0085). Para sacar un duplicado había que tirar abajo la carga completa y
-- volver a subirla sin él, lo que en la práctica significa no sacarlo.
--
-- NO SE BORRA NADA, acá tampoco: el movimiento se queda con `incluido = false`, igual que la
-- reversa, y se puede volver a incluir. Lo que esta migración agrega es el RASTRO: quién lo sacó,
-- cuándo y por qué. Sin el motivo, un movimiento fuera de los libros es indistinguible de un error
-- del sistema, y el que audite el mes no tiene cómo saber si falta plata o si alguien la sacó a
-- propósito.

BEGIN;

ALTER TABLE movimiento_bancario
    -- Por qué se sacó. Es obligatorio en el service, no acá: una fila vieja excluida por la
    -- reversa no tiene motivo propio —lo tiene su importación— y un NOT NULL la rompería.
    ADD COLUMN excluido_motivo text,
    ADD COLUMN excluido_por    uuid REFERENCES usuario(id),
    ADD COLUMN excluido_en     timestamptz;

COMMENT ON COLUMN movimiento_bancario.excluido_motivo IS
  'Por qué este movimiento quedó fuera de los libros a mano. NULL cuando salió por la reversa de su importación (ahí el motivo vive en `importacion.motivo_reversa`).';

-- Los excluidos a mano se consultan para revisarlos y volver a incluirlos; son pocos frente a los
-- 31.000 movimientos, así que el índice va parcial.
CREATE INDEX idx_mov_excluido_a_mano ON movimiento_bancario (empresa_id, excluido_en DESC)
    WHERE NOT incluido AND NOT excluido_por_reversa;

-- El detector de duplicados compara por cuenta + fecha + monto. Sin este índice, cruzar 31.000
-- movimientos contra sí mismos recorre la tabla entera dos veces.
CREATE INDEX idx_mov_duplicado ON movimiento_bancario (cuenta_bancaria_id, fecha, debito, credito)
    WHERE incluido;

INSERT INTO permiso (codigo, modulo, nombre, descripcion) VALUES
    ('bancos.excluir_movimiento',
     'Bancos',
     'Sacar un movimiento de los libros',
     'Excluir un movimiento suelto —un duplicado del banco— sin revertir toda la carga, con motivo y reversible. No borra nada')
ON CONFLICT (codigo) DO NOTHING;

-- Arranca donde ya está el permiso de revertir una carga: son la misma autoridad —sacar plata de
-- los libros— a dos escalas distintas, y quien puede hacerlo con una importación entera puede
-- hacerlo con una fila. Hoy eso es el Director Financiero.
--
-- Se siembra desde las parejas (empresa, rol) que YA existen en `rol_permiso` y no recorriendo
-- `rol`: los roles base son globales y su `empresa_id` es NULL, mientras que `rol_permiso` lo
-- exige.
INSERT INTO rol_permiso (empresa_id, rol_id, permiso_id)
SELECT DISTINCT rp.empresa_id, rp.rol_id, nuevo.id
FROM rol_permiso rp
JOIN permiso viejo ON viejo.id = rp.permiso_id AND viejo.codigo = 'bancos.revertir_importacion'
CROSS JOIN permiso nuevo
WHERE nuevo.codigo = 'bancos.excluir_movimiento'
ON CONFLICT DO NOTHING;

COMMIT;
