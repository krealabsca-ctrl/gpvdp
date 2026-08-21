-- Asignación de cartera: qué sedes trabaja cada operador de cobros.
--
-- Es la FRONTERA DE DATOS del módulo: sin el permiso cxc.ver_todas_sedes, el usuario
-- solo ve los contratos de las sedes que tiene acá. Se verifica en el servicio, no en la
-- pantalla, para que no se pueda evadir armando la URL a mano.
--
-- Un usuario sin filas y sin el permiso no ve NADA, a propósito: es más seguro que
-- mostrarle la cartera completa por omisión.
CREATE TABLE cxc_usuario_sede (
    empresa_id uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,
    usuario_id uuid NOT NULL REFERENCES usuario(id) ON DELETE CASCADE,
    sede_id    uuid NOT NULL REFERENCES cxc_sede(id) ON DELETE CASCADE,
    creado_en  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (empresa_id, usuario_id, sede_id)
);
CREATE INDEX idx_cxc_usuario_sede_usuario ON cxc_usuario_sede (empresa_id, usuario_id);
