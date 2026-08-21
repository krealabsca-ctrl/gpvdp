# GPVDP ERP — Catálogo de endpoints de la API

**Generado automáticamente** desde `backend/internal/server/router.go`. No se edita a mano:
se regenera cuando cambian las rutas.

Total: **253 rutas**. Prefijo de todas: `/v1`.

## Cómo leer la columna «Permiso»

Cada ruta autenticada está protegida por **un permiso del catálogo RBAC**. El middleware
`tenant.RequirePermiso` lo verifica contra la matriz permiso × rol × empresa, que es
configurable en caliente desde la interfaz. El rol `ADMIN` los tiene todos por diseño.

- Un permiso concreto (`cxp.aprobar`) = solo quien lo tenga en la empresa activa.
- `—` = ruta pública o que solo exige haber iniciado sesión.
- `(sin permiso explícito)` = **revisar**: está dentro del grupo autenticado con empresa,
  pero no declara permiso propio. Cualquier usuario con sesión y empresa puede llamarla.

## Autenticación

Tres pasos, en este orden:

1. `POST /v1/auth/login` con `{email, password}` → `access_token` **sin empresa**.
2. `GET /v1/empresas` con ese token → las empresas del usuario.
3. `POST /v1/auth/select-empresa` con `{empresa_id}` → `access_token` **con empresa**.

Todo lo demás exige el token del paso 3: el `empresa_id` sale de ahí, **nunca** del cuerpo
ni del query string. Es la base del aislamiento entre empresas.


## Salud del servicio

1 rutas.

| Método | Ruta | Permiso |
|---|---|---|
| GET | `/v1/healthz` | — |

## Autenticación y sesión

5 rutas.

| Método | Ruta | Permiso |
|---|---|---|
| POST | `/v1/auth/login` | — |
| POST | `/v1/auth/refresh` | — |
| POST | `/v1/auth/select-empresa` | — |
| POST | `/v1/auth/cambiar-password` | — |
| GET | `/v1/me` | — |

## Empresas (multiempresa)

2 rutas.

| Método | Ruta | Permiso |
|---|---|---|
| GET | `/v1/empresas` | — |
| GET | `/v1/empresas/actual` | `(sin permiso explícito)` |

## Bancos (movimientos, catálogo, conciliación, tesorería)

72 rutas.

| Método | Ruta | Permiso |
|---|---|---|
| GET | `/v1/bancos/cuentas` | `bancos.ver` |
| GET | `/v1/bancos/movimientos` | `bancos.ver` |
| GET | `/v1/bancos/movimientos/resumen` | `bancos.ver` |
| GET | `/v1/bancos/reglas` | `bancos.ver` |
| GET | `/v1/bancos/reglas/sugerencia` | `bancos.ver` |
| GET | `/v1/bancos/clasificacion/resumen` | `bancos.ver` |
| GET | `/v1/bancos/catalogo/conceptos` | `bancos.ver` |
| GET | `/v1/bancos/catalogo/clasificaciones` | `bancos.ver` |
| GET | `/v1/bancos/catalogo/bancos` | `bancos.ver` |
| GET | `/v1/bancos/tipo-cambio/:anio/:mes` | `bancos.ver` |
| GET | `/v1/bancos/tipo-cambio/ultimo-sync` | `bancos.ver` |
| GET | `/v1/bancos/parametros` | `bancos.ver` |
| GET | `/v1/bancos/cuadre` | `bancos.ver` |
| GET | `/v1/bancos/cuadre/arbol` | `bancos.ver` |
| GET | `/v1/bancos/dashboard` | `bancos.ver` |
| GET | `/v1/bancos/analisis/serie-mensual` | `bancos.ver` |
| GET | `/v1/bancos/analisis/calendario` | `bancos.ver` |
| GET | `/v1/bancos/analisis/cuentas` | `bancos.ver` |
| GET | `/v1/bancos/analisis/partidas` | `bancos.ver` |
| GET | `/v1/bancos/movimientos/plantilla-clasificacion` | `bancos.exportar` |
| POST | `/v1/bancos/movimientos/clasificar-excel` | `bancos.clasificar` |
| GET | `/v1/bancos/proyecciones` | `bancos.ver` |
| POST | `/v1/bancos/proyecciones` | `bancos.ver` |
| GET | `/v1/bancos/proyecciones/escenarios` | `bancos.ver` |
| GET | `/v1/bancos/traslados/propuestas` | `bancos.ver` |
| GET | `/v1/bancos/tesoreria` | `bancos.ver` |
| PUT | `/v1/bancos/saldos` | `bancos.saldos` |
| GET | `/v1/bancos/carga` | `bancos.ver` |
| GET | `/v1/bancos/patrones` | `bancos.ver` |
| POST | `/v1/bancos/conciliacion-cxp` | `cxp.tesoreria` |
| GET | `/v1/bancos/conciliacion` | `bancos.ver` |
| POST | `/v1/bancos/conciliacion/partidas` | `bancos.conciliar` |
| DELETE | `/v1/bancos/conciliacion/partidas/:id` | `bancos.conciliar` |
| POST | `/v1/bancos/conciliacion/firmar` | `bancos.conciliar` |
| POST | `/v1/bancos/saldos/revisar` | `bancos.saldos_revisar` |
| GET | `/v1/bancos/periodos/:anio/:mes` | `bancos.ver` |
| POST | `/v1/bancos/importaciones` | `bancos.importar` |
| GET | `/v1/bancos/importaciones/:id/preview` | `bancos.importar` |
| POST | `/v1/bancos/importaciones/:id/confirmar` | `bancos.importar` |
| PATCH | `/v1/bancos/movimientos/:id/clasificacion` | `bancos.clasificar` |
| POST | `/v1/bancos/movimientos/clasificar-masivo` | `bancos.clasificar` |
| POST | `/v1/bancos/reglas` | `bancos.reglas` |
| PATCH | `/v1/bancos/reglas/:id` | `bancos.reglas` |
| DELETE | `/v1/bancos/reglas/:id` | `bancos.reglas` |
| GET | `/v1/bancos/catalogo/diccionario` | `bancos.exportar` |
| POST | `/v1/bancos/catalogo/diccionario` | `bancos.reglas` |
| POST | `/v1/bancos/catalogo/conceptos` | `bancos.catalogo` |
| PATCH | `/v1/bancos/catalogo/conceptos/:id` | `bancos.catalogo` |
| DELETE | `/v1/bancos/catalogo/conceptos/:id` | `bancos.catalogo` |
| POST | `/v1/bancos/catalogo/clasificaciones` | `bancos.catalogo` |
| PATCH | `/v1/bancos/catalogo/clasificaciones/:id` | `bancos.catalogo` |
| DELETE | `/v1/bancos/catalogo/clasificaciones/:id` | `bancos.catalogo` |
| POST | `/v1/bancos/catalogo/bancos` | `bancos.catalogo` |
| PATCH | `/v1/bancos/catalogo/bancos/:id` | `bancos.catalogo` |
| POST | `/v1/bancos/catalogo/cuentas` | `bancos.catalogo` |
| PATCH | `/v1/bancos/catalogo/cuentas/:id` | `bancos.catalogo` |
| DELETE | `/v1/bancos/catalogo/bancos/:id` | `bancos.catalogo` |
| POST | `/v1/bancos/catalogo/bancos/:id/activo` | `bancos.catalogo` |
| GET | `/v1/bancos/catalogo/cuentas/:id/uso` | `bancos.ver` |
| DELETE | `/v1/bancos/catalogo/cuentas/:id` | `bancos.catalogo` |
| POST | `/v1/bancos/catalogo/cuentas/:id/activo` | `bancos.catalogo` |
| POST | `/v1/bancos/catalogo/conceptos/:id/fusionar` | `bancos.catalogo` |
| POST | `/v1/bancos/catalogo/clasificaciones/:id/fusionar` | `bancos.catalogo` |
| POST | `/v1/bancos/cotizaciones` | `bancos.tc_registrar` |
| POST | `/v1/bancos/tipo-cambio/sync` | `bancos.tc_registrar` |
| POST | `/v1/bancos/tipo-cambio/:anio/:mes/congelar` | `bancos.tc_congelar` |
| PATCH | `/v1/bancos/parametros/tolerancia` | `bancos.ajustes` |
| GET | `/v1/bancos/exportaciones/movimientos` | `bancos.exportar` |
| GET | `/v1/bancos/exportaciones/cuadre` | `bancos.exportar` |
| POST | `/v1/bancos/traslados/emparejar` | `bancos.traslados` |
| POST | `/v1/bancos/traslados/desemparejar` | `bancos.traslados` |
| POST | `/v1/bancos/periodos/:anio/:mes/cerrar` | `bancos.cerrar_periodo` |

## Cuentas por Pagar

72 rutas.

| Método | Ruta | Permiso |
|---|---|---|
| GET | `/v1/cxp/dashboard` | `cxp.dashboard` |
| GET | `/v1/cxp/bandeja` | `cxp.ver` |
| GET | `/v1/cxp/catalogo/subclasificaciones` | `cxp.ver` |
| GET | `/v1/cxp/departamentos` | `cxp.ver` |
| GET | `/v1/cxp/departamentos/:id/validadores` | `cxp.ver` |
| GET | `/v1/cxp/usuarios` | `cxp.departamentos` |
| GET | `/v1/cxp/proveedores` | `cxp.ver` |
| GET | `/v1/cxp/proveedores/:id` | `cxp.ver` |
| GET | `/v1/cxp/proveedores/:id/gastos` | `cxp.ver` |
| GET | `/v1/cxp/proveedores/sin-iban` | `cxp.ver` |
| GET | `/v1/cxp/documentos` | `cxp.ver` |
| GET | `/v1/cxp/documentos/:id` | `cxp.ver` |
| GET | `/v1/cxp/documentos/:id/historial` | `cxp.ver` |
| GET | `/v1/cxp/documentos/:id/comprobante` | `cxp.ver` |
| GET | `/v1/cxp/documentos/:id/anticipos` | `cxp.ver` |
| GET | `/v1/cxp/anticipos/disponibles` | `cxp.ver` |
| GET | `/v1/cxp/anticipos` | `cxp.ver` |
| GET | `/v1/cxp/lotes` | `cxp.ver` |
| POST | `/v1/cxp/catalogo/subclasificaciones` | `cxp.clasificar` |
| PATCH | `/v1/cxp/documentos/:id/clasificacion` | `cxp.clasificar` |
| POST | `/v1/cxp/documentos/clasificar-masivo` | `cxp.clasificar` |
| POST | `/v1/cxp/documentos/tipo-masivo` | `cxp.clasificar` |
| POST | `/v1/cxp/documentos/prioridad-masiva` | `cxp.clasificar` |
| POST | `/v1/cxp/documentos/:id/revisar` | `cxp.revisar` |
| POST | `/v1/cxp/documentos/transicion-masiva` | `cxp.revisar` |
| PATCH | `/v1/cxp/documentos/:id/departamento` | `cxp.clasificar` |
| POST | `/v1/cxp/documentos/:id/validar-depto` | `cxp.validar_depto` |
| POST | `/v1/cxp/documentos/:id/validar-escalado` | `cxp.validar_escalado` |
| POST | `/v1/cxp/documentos/:id/devolver` | `cxp.validar_depto` |
| POST | `/v1/cxp/documentos/:id/anticipos` | `cxp.anticipos` |
| POST | `/v1/cxp/documentos/:id/anticipos/lote` | `cxp.anticipos` |
| DELETE | `/v1/cxp/documentos/:id/anticipos/:aplicacionId` | `cxp.anticipos` |
| POST | `/v1/cxp/documentos/:id/aprobar` | `cxp.aprobar` |
| POST | `/v1/cxp/documentos/:id/aprobar-contabilidad` | `cxp.aprobar_contabilidad` |
| PATCH | `/v1/cxp/documentos/:id/contabilidad` | `cxp.marcar_contabilidad` |
| PATCH | `/v1/cxp/proveedores/:id/contabilidad` | `cxp.marcar_contabilidad` |
| PATCH | `/v1/cxp/contabilidad/conceptos/:id` | `cxp.marcar_contabilidad` |
| PATCH | `/v1/cxp/contabilidad/clasificaciones/:id` | `cxp.marcar_contabilidad` |
| GET | `/v1/cxp/contabilidad/marcas` | `cxp.ver` |
| GET | `/v1/cxp/parametros` | `cxp.ver` |
| PUT | `/v1/cxp/parametros/:clave` | `cxp.parametros` |
| POST | `/v1/cxp/documentos/:id/programar` | `cxp.tesoreria` |
| POST | `/v1/cxp/documentos/:id/pagar` | `cxp.tesoreria` |
| POST | `/v1/cxp/documentos/:id/conciliar` | `cxp.tesoreria` |
| GET | `/v1/cxp/pagos/archivo` | `cxp.tesoreria` |
| POST | `/v1/cxp/pagos/archivo` | `cxp.tesoreria` |
| POST | `/v1/cxp/conciliacion/match` | `cxp.tesoreria` |
| POST | `/v1/cxp/lotes` | `cxp.tesoreria` |
| GET | `/v1/cxp/lotes/:id/macro` | `cxp.tesoreria` |
| POST | `/v1/cxp/documentos/:id/comprobante` | `cxp.comprobante` |
| POST | `/v1/cxp/documentos/:id/comprobante/enviar` | `cxp.comprobante` |
| POST | `/v1/cxp/proveedores` | `cxp.proveedores` |
| PATCH | `/v1/cxp/proveedores/:id` | `cxp.proveedores` |
| POST | `/v1/cxp/proveedores/:id/desactivar` | `cxp.proveedores` |
| POST | `/v1/cxp/proveedores/iban/preview` | `cxp.proveedores` |
| POST | `/v1/cxp/proveedores/iban` | `cxp.proveedores` |
| POST | `/v1/cxp/departamentos` | `cxp.departamentos` |
| PATCH | `/v1/cxp/departamentos/:id` | `cxp.departamentos` |
| POST | `/v1/cxp/departamentos/:id/desactivar` | `cxp.departamentos` |
| POST | `/v1/cxp/departamentos/:id/validadores` | `cxp.departamentos` |
| DELETE | `/v1/cxp/departamentos/:id/validadores/:usuarioId` | `cxp.departamentos` |
| GET | `/v1/cxp/cajas` | `cxp.caja_ver` |
| POST | `/v1/cxp/cajas` | `cxp.caja_administrar` |
| PATCH | `/v1/cxp/cajas/:id` | `cxp.caja_administrar` |
| POST | `/v1/cxp/cajas/:id/desactivar` | `cxp.caja_administrar` |
| GET | `/v1/cxp/cajas/:id/vales` | `cxp.caja_ver` |
| POST | `/v1/cxp/cajas/:id/vales` | `cxp.caja_vale` |
| POST | `/v1/cxp/cajas/:id/vales/:valeId/anular` | `cxp.caja_vale` |
| POST | `/v1/cxp/cajas/:id/reposicion` | `cxp.caja_reponer` |
| POST | `/v1/cxp/documentos` | `cxp.importar` |
| POST | `/v1/cxp/importaciones` | `cxp.importar` |
| POST | `/v1/cxp/importaciones/confirmar` | `cxp.importar` |

## Cuentas por Cobrar

42 rutas.

| Método | Ruta | Permiso |
|---|---|---|
| GET | `/v1/cxc/catalogos` | `cxc.ver` |
| GET | `/v1/cxc/contratos` | `cxc.ver` |
| GET | `/v1/cxc/contratos/:numero` | `cxc.ver` |
| GET | `/v1/cxc/cargos/plan` | `cxc.ver` |
| POST | `/v1/cxc/cargos/generar` | `cxc.importar` |
| POST | `/v1/cxc/importaciones/contratos/previsualizar` | `cxc.importar` |
| POST | `/v1/cxc/importaciones/contratos/confirmar` | `cxc.importar` |
| GET | `/v1/cxc/cobros` | `cxc.ver` |
| GET | `/v1/cxc/asociaciones/panorama` | `cxc.ver` |
| POST | `/v1/cxc/cobros` | `cxc.cobros` |
| POST | `/v1/cxc/cobros/:id/reversar` | `cxc.aplicar` |
| POST | `/v1/cxc/cobros/:id/identificar` | `cxc.aplicar` |
| POST | `/v1/cxc/importaciones/cobros/previsualizar` | `cxc.importar` |
| POST | `/v1/cxc/importaciones/cobros/confirmar` | `cxc.cobros` |
| GET | `/v1/cxc/cola` | `cxc.ver` |
| GET | `/v1/cxc/gestiones/catalogos` | `cxc.ver` |
| GET | `/v1/cxc/contratos/:numero/gestiones` | `cxc.ver` |
| POST | `/v1/cxc/gestiones` | `cxc.gestionar` |
| GET | `/v1/cxc/parametros` | `cxc.ver` |
| PUT | `/v1/cxc/parametros` | `cxc.parametros` |
| PATCH | `/v1/cxc/tramos/:codigo` | `cxc.parametros` |
| PATCH | `/v1/cxc/formas-pago/:id` | `cxc.parametros` |
| POST | `/v1/cxc/sedes` | `cxc.parametros` |
| PATCH | `/v1/cxc/sedes/:id` | `cxc.parametros` |
| PUT | `/v1/cxc/usuarios/:id/sedes` | `cxc.parametros` |
| GET | `/v1/cxc/asociaciones/:id/planilla` | `cxc.ver` |
| POST | `/v1/cxc/asociaciones/:id/planilla` | `cxc.cobros` |
| GET | `/v1/cxc/planillas/:id/candidatos` | `cxc.ver` |
| POST | `/v1/cxc/planillas/:id/depositos` | `cxc.aplicar` |
| DELETE | `/v1/cxc/planillas/:id/depositos/:movimiento` | `cxc.aplicar` |
| GET | `/v1/cxc/notas-credito` | `cxc.ver` |
| POST | `/v1/cxc/notas-credito` | `cxc.notas_credito` |
| POST | `/v1/cxc/notas-credito/:id/anular` | `cxc.notas_credito` |
| GET | `/v1/cxc/contratos/:numero/suspension` | `cxc.ver` |
| POST | `/v1/cxc/contratos/:numero/suspender` | `cxc.suspender` |
| POST | `/v1/cxc/contratos/:numero/reactivar` | `cxc.suspender` |
| GET | `/v1/cxc/arreglos` | `cxc.ver` |
| GET | `/v1/cxc/arreglos/:id` | `cxc.ver` |
| POST | `/v1/cxc/arreglos` | `cxc.gestionar` |
| POST | `/v1/cxc/arreglos/:id/quebrar` | `cxc.arreglos` |
| POST | `/v1/cxc/arreglos/:id/anular` | `cxc.arreglos` |
| GET | `/v1/cxc/preventivo` | `cxc.preventivo` |

## RRHH / Nómina

43 rutas.

| Método | Ruta | Permiso |
|---|---|---|
| GET | `/v1/rrhh/dashboard` | `rrhh.ver` |
| GET | `/v1/rrhh/empleados` | `rrhh.ver` |
| GET | `/v1/rrhh/empleados/:id` | `rrhh.ver` |
| POST | `/v1/rrhh/empleados` | `rrhh.empleados` |
| PATCH | `/v1/rrhh/empleados/:id` | `rrhh.empleados` |
| POST | `/v1/rrhh/empleados/:id/desactivar` | `rrhh.empleados` |
| GET | `/v1/rrhh/empleados/:id/deducciones` | `rrhh.ver` |
| POST | `/v1/rrhh/empleados/:id/deducciones` | `rrhh.empleados` |
| PATCH | `/v1/rrhh/empleados/:id/deducciones/:dedId` | `rrhh.empleados` |
| POST | `/v1/rrhh/empleados/:id/deducciones/:dedId/desactivar` | `rrhh.empleados` |
| GET | `/v1/rrhh/corridas` | `rrhh.ver` |
| GET | `/v1/rrhh/corridas/:id` | `rrhh.ver` |
| POST | `/v1/rrhh/corridas` | `rrhh.corrida` |
| PUT | `/v1/rrhh/corridas/:id/novedades` | `rrhh.corrida` |
| POST | `/v1/rrhh/corridas/:id/recalcular` | `rrhh.corrida` |
| POST | `/v1/rrhh/corridas/:id/aprobar` | `rrhh.corrida` |
| POST | `/v1/rrhh/corridas/:id/pagar` | `rrhh.corrida` |
| POST | `/v1/rrhh/corridas/:id/anular` | `rrhh.corrida` |
| POST | `/v1/rrhh/corridas/:id/boletas` | `rrhh.corrida` |
| POST | `/v1/rrhh/vacaciones/:id/aviso` | `rrhh.ausencias` |
| GET | `/v1/rrhh/corridas/:id/archivo-pago` | `rrhh.corrida` |
| GET | `/v1/rrhh/corridas/:id/planilla-ccss` | `rrhh.ver` |
| GET | `/v1/rrhh/finiquitos` | `rrhh.ver` |
| GET | `/v1/rrhh/finiquitos/:id` | `rrhh.ver` |
| POST | `/v1/rrhh/finiquitos` | `rrhh.finiquito` |
| PATCH | `/v1/rrhh/finiquitos/:id` | `rrhh.finiquito` |
| POST | `/v1/rrhh/finiquitos/:id/aprobar` | `rrhh.finiquito` |
| POST | `/v1/rrhh/finiquitos/:id/pagar` | `rrhh.finiquito` |
| POST | `/v1/rrhh/finiquitos/:id/anular` | `rrhh.finiquito` |
| GET | `/v1/rrhh/incapacidades` | `rrhh.ver` |
| POST | `/v1/rrhh/incapacidades` | `rrhh.ausencias` |
| POST | `/v1/rrhh/incapacidades/:id/anular` | `rrhh.ausencias` |
| GET | `/v1/rrhh/vacaciones/saldos` | `rrhh.ver` |
| GET | `/v1/rrhh/vacaciones` | `rrhh.ver` |
| POST | `/v1/rrhh/vacaciones` | `rrhh.ausencias` |
| POST | `/v1/rrhh/vacaciones/:id/anular` | `rrhh.ausencias` |
| GET | `/v1/rrhh/reportes/provisiones` | `rrhh.ver` |
| GET | `/v1/rrhh/parametros/:anio` | `rrhh.ver` |
| PUT | `/v1/rrhh/parametros/:anio` | `rrhh.parametros` |
| GET | `/v1/rrhh/conceptos` | `rrhh.ver` |
| POST | `/v1/rrhh/conceptos` | `rrhh.parametros` |
| PATCH | `/v1/rrhh/conceptos/:id` | `rrhh.parametros` |
| POST | `/v1/rrhh/conceptos/:id/desactivar` | `rrhh.parametros` |

## Usuarios, roles y permisos

12 rutas.

| Método | Ruta | Permiso |
|---|---|---|
| GET | `/v1/rbac/mis-permisos` | `(sin permiso explícito)` |
| GET | `/v1/rbac/permisos` | `admin.roles` |
| GET | `/v1/rbac/roles` | `admin.roles` |
| GET | `/v1/rbac/matriz` | `admin.roles` |
| PUT | `/v1/rbac/roles/:codigo/permisos` | `admin.roles` |
| POST | `/v1/rbac/roles` | `admin.roles` |
| GET | `/v1/rbac/usuarios` | `admin.roles` |
| POST | `/v1/rbac/usuarios` | `admin.roles` |
| PATCH | `/v1/rbac/usuarios/:id` | `admin.roles` |
| POST | `/v1/rbac/usuarios/:id/reset-password` | `admin.roles` |
| DELETE | `/v1/rbac/usuarios/:id/acceso` | `admin.roles` |
| POST | `/v1/rbac/permisos/aplicar-faltantes` | `admin.roles` |

## Plantillas de correo

4 rutas.

| Método | Ruta | Permiso |
|---|---|---|
| GET | `/v1/plantillas` | `admin.plantillas` |
| PUT | `/v1/plantillas/:clave` | `admin.plantillas` |
| DELETE | `/v1/plantillas/:clave` | `admin.plantillas` |
| POST | `/v1/plantillas/:clave/vista-previa` | `admin.plantillas` |

## Rutas sin permiso explícito (2)

Estas rutas están detrás de sesión + empresa, pero no declaran un permiso propio.
Vale revisar una por una si eso es intencional:

- GET `/v1/empresas/actual`
- GET `/v1/rbac/mis-permisos`
