-- 0097 · El convenio con el proveedor manda, y la llave para abrir el candado del contado.
--
-- DOS PEDIDOS DEL DIRECTOR FINANCIERO (12-oct-2026), que son el mismo problema en dos tiempos.
--
-- EL PROBLEMA. Hay proveedores que declaran «contado» en el comprobante aunque el convenio real
-- sea a crédito. El importador toma esa palabra al pie de la letra y la factura nace BLOQUEADA
-- para pago —un candado contra el doble pago, porque una compra de contado suele estar ya pagada
-- por caja chica y pagarla otra vez por banco saca el dinero dos veces—. Pero cuando el emisor se
-- equivocó, esa factura queda trancada para siempre: «quedaría sucio el sistema y facturas sin
-- pagar cuando deben pagarse».
--
-- 1 · HACIA ADELANTE: `plazo_manda` pasa a llamarse `convenio_manda` y decide DOS cosas.
--
-- Ya decidía de dónde sale el vencimiento (mig 0095). Ahora decide también si se le cree al
-- comprobante o al convenio cuando declaran condiciones de venta distintas. Es UNA pregunta de
-- negocio —«¿a quién le creo sobre este proveedor?»— y por eso es UN switch: dos casillas
-- separadas para la misma pregunta es lo que vuelve inusable una pantalla.
--
-- Se renombra la columna en vez de agregar otra porque el nombre viejo pasaría a mentir. Y el
-- renombrado no mueve ni un dato: `plazo_manda` está en 'XML' en los 649 proveedores, que es el
-- default y lo que el sistema venía haciendo.
--
-- 2 · HACIA ATRÁS: la huella para liberar lo que ya está trancado.
--
-- `bloqueado_para_pago` tenía DOCE lectores y un solo escritor —el importador, que lo pone en
-- true—. El comentario del código decía «no puede llegar al archivo de pagos sin que una persona
-- la libere» y esa persona no tenía con qué: el candado no tenía llave. Es exactamente lo que ya
-- pasó con `incluido` en Bancos (mig 0093).
--
-- Liberar NO borra el bloqueo de la historia: deja quién, cuándo y POR QUÉ. Sin el motivo, dentro
-- de seis meses nadie puede distinguir la factura que el emisor marcó mal de la que alguien
-- liberó por error y se pagó dos veces.

-- ── 1 · El convenio manda ────────────────────────────────────────────────────────────────
ALTER TABLE proveedor RENAME COLUMN plazo_manda TO convenio_manda;
ALTER TABLE proveedor RENAME CONSTRAINT proveedor_plazo_manda_check TO proveedor_convenio_manda_check;

COMMENT ON COLUMN proveedor.convenio_manda IS
  'XML: manda el comprobante (default, lo de siempre). PROVEEDOR: manda el convenio de la ficha, '
  'tanto para el plazo de vencimiento como para la condición de venta (contado/crédito).';

-- ── 2 · La huella de la liberación ───────────────────────────────────────────────────────
ALTER TABLE documento_cxp
    ADD COLUMN IF NOT EXISTS desbloqueado_por    uuid REFERENCES usuario(id),
    ADD COLUMN IF NOT EXISTS desbloqueado_en     timestamptz,
    ADD COLUMN IF NOT EXISTS desbloqueo_motivo   text;

-- Buscar lo liberado es una pregunta de auditoría frecuente («¿qué facturas de contado se
-- terminaron pagando por banco y por qué?»), y son pocas filas sobre una tabla grande.
CREATE INDEX IF NOT EXISTS idx_docxp_liberadas
    ON documento_cxp (empresa_id, desbloqueado_en DESC)
    WHERE desbloqueado_en IS NOT NULL;

-- ── 3 · El permiso ───────────────────────────────────────────────────────────────────────
--
-- Permiso PROPIO y no `cxp.tesoreria`: liberar es afirmar «esta factura NO se pagó por caja chica
-- ni por tarjeta, se paga por banco». Es la única barrera contra el doble pago que queda en pie
-- una vez que la factura entró, así que quién puede levantarla tiene que ser una decisión
-- separada de quién arma el archivo de pagos.
INSERT INTO permiso (codigo, modulo, nombre, descripcion) VALUES
    ('cxp.liberar_pago',
     'Cuentas por pagar',
     'Liberar una factura de contado para pagarla por banco',
     'Levanta el candado anti-doble-pago de una factura que el emisor declaró de contado. Exige motivo y queda registrado quién lo hizo.')
ON CONFLICT (codigo) DO NOTHING;

-- Se siembra sobre las parejas (empresa, rol) QUE YA EXISTEN en `rol_permiso`: los roles base son
-- globales (`empresa_id` NULL) y `rol_permiso` lo exige NOT NULL. Lo recibe quien ya podía
-- APROBAR el pago: es la misma firma —decir que este gasto sale por el banco— y no se le estrena
-- la facultad a nadie que no la tuviera en otra forma.
INSERT INTO rol_permiso (empresa_id, rol_id, permiso_id)
SELECT DISTINCT rp.empresa_id, rp.rol_id, p.id
FROM rol_permiso rp
JOIN permiso apr ON apr.id = rp.permiso_id AND apr.codigo = 'cxp.aprobar'
CROSS JOIN permiso p
WHERE p.codigo = 'cxp.liberar_pago'
ON CONFLICT DO NOTHING;
