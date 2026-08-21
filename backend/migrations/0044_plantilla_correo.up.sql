-- Plantillas de correo personalizables por empresa.
--
-- Hasta ahora el correo del comprobante de pago estaba escrito dentro del código (asunto y
-- cuerpo fijos, con la firma «Finance Group VDP» y el símbolo ₡ aunque la factura fuera en
-- dólares). Cada empresa del grupo se comunica distinto con sus proveedores y su gente, así que
-- el texto es CONFIGURACIÓN, no código.
--
-- Solo se guarda lo que el usuario cambió: si no hay fila, rige el texto por defecto que vive en
-- el catálogo de tipos (plantillas.go). Así una empresa nueva funciona sin sembrar nada.
CREATE TABLE plantilla_correo (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id      uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,
    -- clave del tipo de notificación (CXP_COMPROBANTE, NOMINA_BOLETA, NOMINA_VACACIONES…).
    -- El catálogo de claves vive en el código; acá no se valida con CHECK para poder agregar
    -- tipos nuevos sin migración.
    clave           text NOT NULL,
    asunto          text NOT NULL,
    cuerpo          text NOT NULL,
    actualizado_por uuid REFERENCES usuario(id),
    actualizado_en  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (empresa_id, clave)
);
