-- «Mi presupuesto»: que un líder de área vea su gasto sin entrar a Bancos (16-nov-2026).
--
-- ── EL PEDIDO ────────────────────────────────────────────────────────────────────────────────
--
-- «Debo permitirle ver a los líderes, gerencias, jefaturas cómo van sus gastos o los ingresos del
-- mes según corresponda, pero sin darle acceso al módulo de bancos… aunque el tema de los accesos
-- no va tan ligado al perfil sino al usuario.»
--
-- Esta migración trae la mitad de GASTO. La de ingreso espera una decisión: su presupuesto es por
-- PARTIDA y no por departamento, y hoy CERO movimientos de ingreso tienen departamento asignado
-- (10.818 movimientos, ₡565,9 M), así que el eje tiene que definirse antes de construirla.
--
-- ── POR QUÉ NO HAY TABLA NUEVA ───────────────────────────────────────────────────────────────
--
-- El alcance por USUARIO ya existe: `departamento_validador` (departamento_id, usuario_id, rol)
-- liga personas a áreas y la usa CxP para la validación de área. Crear una segunda tabla para lo
-- mismo obligaría a mantener dos listas en sincronía y a explicar por qué un jefe aparece en una
-- y no en la otra. Se reusa.
--
-- Nota: «Mi partida» de Bancos usa el modelo CONTRARIO —alcance por ROL, en
-- `rol_clasificacion_consulta`— porque así se decidió en setiembre. Los dos conviven: son ejes
-- distintos (partida vs. departamento) y el usuario pidió explícitamente que este vaya al usuario.

INSERT INTO permiso (codigo, modulo, nombre, descripcion, critico, orden)
VALUES ('bancos.ver_mi_presupuesto', 'bancos', 'Ver el presupuesto de mi área',
        'Abre «Mi presupuesto»: el gasto del mes contra el presupuesto, SOLO de las áreas donde la persona está declarada como responsable. No da acceso al módulo de Bancos ni a los movimientos.',
        false, 0)
ON CONFLICT (codigo) DO NOTHING;

-- NO se reparte a ningún rol, a propósito.
--
-- Es el mismo criterio que `bancos.ver_mi_segmento` (mig 0077): es una capacidad para los roles de
-- consulta que el negocio cree, no algo que deba heredar nadie por ser financiero. Y el permiso
-- solo no abre nada: sin filas en `departamento_validador` la pantalla no muestra ni una línea,
-- porque el alcance vacío CIERRA.
