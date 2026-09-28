-- ⚠ ESTE DOWN DESTRUYE SECRETOS Y AUDITORÍA.
--
-- `correo_saliente` guarda las contraseñas de los buzones de las tres empresas. Al bajar esta
-- migración se van, y volver a subirla NO las recupera: hay que escribirlas de nuevo a mano, una
-- por empresa. `comprobante_envio` es la única evidencia de a quién se le mandó cada comprobante
-- y con qué resultado; borrarla no se deshace.
--
-- Se quitan los grants antes que el permiso (FK) y las tablas en orden inverso al de creación.
--
-- `documento_cxp.comprobante_enviado_en` NO se toca acá: es de una migración anterior y esta no la
-- modificó. Lo que sí queda revertido por código (no por SQL) es que el reemplazo del adjunto la
-- ponga en NULL.

DELETE FROM rol_permiso WHERE permiso_id IN (SELECT id FROM permiso WHERE codigo = 'admin.correo');
DELETE FROM permiso WHERE codigo = 'admin.correo';

DROP TABLE IF EXISTS comprobante_envio;
DROP TABLE IF EXISTS correo_saliente;
