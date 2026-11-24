-- «Mi presupuesto», mitad de INGRESO: por partida y por usuario (16-nov-2026).
--
-- ── POR QUÉ EL EJE ES LA PARTIDA Y NO EL DEPARTAMENTO ────────────────────────────────────────
--
-- Decisión del Director Financiero, y los datos la respaldan. Las 18 partidas de ingreso de Valle
-- de Paz son CANALES DE COBRO, no áreas:
--
--   Asociaciones / Cooperativas  ₡251,4 M      Depósito de Clientes  ₡90,3 M
--   Datáfonos                    ₡79,6 M       Caja de Ingresos      ₡39,1 M
--   Cargo BN / BAC / BP / Promerica            Cobrador Limón / Cartago / Puntarenas
--
-- «Depósito de Clientes» no es de un departamento: es por dónde entra la plata. Forzar una
-- atribución a área inventaría un dueño que el negocio no tiene, y además el presupuesto de
-- ingreso YA está modelado por clasificación (mig 0116). El eje de la plata y el eje del
-- presupuesto tienen que ser el mismo o el porcentaje no significa nada.
--
-- Verificado: de 18 partidas de ingreso, CERO tienen departamento asignado.
--
-- ── POR QUÉ UNA TABLA NUEVA Y NO SE EXTIENDE LA DE ROL ───────────────────────────────────────
--
-- `rol_clasificacion_consulta` (mig 0077) hace lo mismo pero por ROL, y el usuario pidió que
-- ESTE acceso vaya a la PERSONA. Se evaluó agregarle `usuario_id` nullable: los `JOIN rol`
-- internos de las cuatro consultas de «Mi partida» descartarían solas las filas sin rol, así que
-- técnicamente no rompería. Se descartó igual porque obliga a volver `rol_id` nullable en una
-- tabla en uso, y el ahorro —una tabla de tres columnas— no paga ese riesgo.
--
-- QUEDA DICHO, NO ESCONDIDO: con esta son TRES formas de decir «quién ve qué».
--   · rol_clasificacion_consulta   rol     × partida       «Mi partida»
--   · departamento_validador       usuario × departamento  «Mi presupuesto» (gasto, mig 0124)
--   · usuario_clasificacion_consulta usuario × partida     «Mi presupuesto» (ingreso, esta)
-- Conviene unificarlas el día que se toque este tema de nuevo; hoy unificarlas sería reescribir
-- tres pantallas que funcionan.

CREATE TABLE IF NOT EXISTS usuario_clasificacion_consulta (
    empresa_id       uuid        NOT NULL REFERENCES empresa (id) ON DELETE CASCADE,
    usuario_id       uuid        NOT NULL REFERENCES usuario (id) ON DELETE CASCADE,
    clasificacion_id uuid        NOT NULL REFERENCES clasificacion (id) ON DELETE CASCADE,
    creado_en        timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (empresa_id, usuario_id, clasificacion_id)
);

-- Los dos índices espejan los de la tabla de rol: se consulta por usuario (qué ve esta persona)
-- y por partida (quién ve esta partida, para la pantalla de administración).
CREATE INDEX IF NOT EXISTS idx_ucc_usuario ON usuario_clasificacion_consulta (empresa_id, usuario_id);
CREATE INDEX IF NOT EXISTS idx_ucc_clasificacion ON usuario_clasificacion_consulta (empresa_id, clasificacion_id);

-- El permiso.
--
-- Separado de `bancos.ver_mi_presupuesto` (el de gasto) porque el usuario dijo que «en algunos
-- casos aplica solo para gastos, en otras ingresos también»: con un permiso único no habría forma
-- de dar uno sin el otro. Quien tenga los dos ve las dos secciones en la misma pantalla.
INSERT INTO permiso (codigo, modulo, nombre, descripcion, critico, orden)
VALUES ('bancos.ver_mi_ingreso', 'bancos', 'Ver el ingreso de mis partidas',
        'Abre la sección de ingreso de «Mi presupuesto»: lo que entró en las partidas asignadas a la persona, contra la meta. No da acceso al módulo de Bancos ni a los movimientos.',
        false, 0)
ON CONFLICT (codigo) DO NOTHING;

-- No se reparte a ningún rol, igual que el de gasto y que `bancos.ver_mi_segmento`: es una
-- capacidad para los roles de consulta que el negocio cree. Y el permiso solo no abre nada —sin
-- filas en `usuario_clasificacion_consulta` la sección no muestra una línea, porque el alcance
-- vacío CIERRA.
