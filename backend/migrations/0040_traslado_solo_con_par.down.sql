-- No hay vuelta atrás de datos: volver a marcar como traslado lo que no tiene pareja sería
-- reintroducir el error (excluir del EBITDA sin contraparte). La bandera se reconstruye sola
-- al emparejar. Se deja explícito para que el rollback no falle.
SELECT 1;
