# MÓDULO BANCOS — Especificación Funcional Enterprise
### Finance Group GPVDP · v1.0 · Comité de Arquitectura

> **Estado:** Borrador para validación del Director Financiero.
> **Ámbito:** Multiempresa (Valle de Paz, Coopeprofa, Memorial Pets · escalable), aislamiento por `tenant_id`, catálogos independientes por empresa.
> **Stack objetivo:** Go + Gin · React + TypeScript · PostgreSQL.
> **Convención de moneda base:** Colón costarricense (CRC). Movimientos USD convertidos a CRC con el motor de tipo de cambio (§7.3).
> **TODO (insumos del negocio):** [T1] muestras de estado de cuenta por banco · [T2] fórmula/perímetro de EBITDA · [T3] matriz fina de roles.

---

## 1. Objetivo del módulo

Centralizar el pulso financiero diario de cada empresa: importar sin fricción los estados de cuenta de todos los bancos (CRC y USD), clasificar automáticamente cada movimiento por Concepto y Clasificación, cuadrar ingresos y gastos por período, y alimentar dashboards, proyecciones y el reporte de accionistas con información íntegra, auditable y trazable al 100% de los movimientos.

El módulo es la **fuente de verdad de caja** del grupo y la base sobre la que se concilian Cuentas por Pagar y se nutre Presupuesto.

## 2. Alcance

**Incluye:**
- Importador universal de estados de cuenta descargables en Excel de bancos de Costa Rica.
- Memoria de banco por IBAN, detección de inicio de movimientos y descarte de residuos.
- Motor de tipo de cambio mensual con congelamiento al cierre (§7.3).
- Motor de clasificación con reglas editables, palabras clave, aprendizaje y umbral de confianza.
- Detección y gestión de duplicados (reales vs. reimportación).
- Emparejamiento de Traslados de Fondos y Overnight entre cuentas del grupo.
- Pantallas: Importador, Catálogo, Revisar, Movimientos, Cuadre, Exportar, Dashboard, Proyecciones.
- Exportaciones parametrizables (incluye extracción de consecutivo largo Davivienda).

**No incluye (fuera de este módulo):**
- Registro contable formal / NIIF (capa futura; el modelo queda preparado, §25).
- Ejecución de pagos (vive en CxP y Tesorería).
- Presupuesto y Nómina (módulos propios).

## 3. Procesos de negocio

1. **Ingesta diaria:** descarga del estado de cuenta → importación → previsualización → confirmación.
2. **Normalización:** identificación de banco (IBAN), detección de encabezados, discriminación CRC/USD, conversión USD→CRC.
3. **Clasificación:** el motor asigna Concepto + Clasificación cuando la confianza ≥ 90%; el resto cae en "No identificado".
4. **Revisión:** el usuario clasifica lo no identificado, crea reglas por bloque y confirma emparejamientos de traslados/overnight.
5. **Cuadre diario y de cierre:** validación por concepto; base del reporte de accionistas y estados.
6. **Exportación y distribución:** reportes a equipos (Asociaciones, Depósito, Servicios de Emergencia) y a gerencia.
7. **Proyección:** estimación de cierre de mes por líneas de ingreso, con metas de crecimiento parametrizables.

## 4. Actores involucrados

| Actor | Rol en Bancos |
|---|---|
| Auxiliar Financiero | Importa, clasifica en Revisar, ejecuta cuadre diario. |
| Supervisor Financiero | Valida clasificación, aprueba reglas nuevas, revisa cuadre. |
| Director Financiero | Consume dashboard, proyecciones y reporte de accionistas; parametriza metas. |
| Gerencia General | Vista consolidada de lectura, alertas estratégicas. |
| Auditor Interno | Solo lectura + trazabilidad y logs. |
| Sistema (motores) | Importa, clasifica, concilia, sincroniza TC, dispara alertas. |

> [T3] La matriz fina rol × acción × empresa se adjunta como anexo cuando se defina; el módulo la lee de forma dinámica (roles ajustables e incorporables).

## 5. Casos de uso

- **CU-01 Importar estado de cuenta:** el usuario sube un Excel; el sistema identifica banco/IBAN, detecta el inicio de movimientos, discrimina moneda, previsualiza y marca duplicados; el usuario confirma.
- **CU-02 Identificar banco por IBAN en memoria:** al reconocer un IBAN ya visto, el sistema asigna el banco automáticamente y evita duplicar la cuenta.
- **CU-03 Convertir USD a CRC:** el sistema aplica el TC del mes (provisional o congelado) al movimiento en dólares.
- **CU-04 Clasificar automáticamente:** el motor asigna Concepto/Clasificación con confianza ≥ 90%.
- **CU-05 Revisar no identificados:** el usuario filtra, detecta patrones, clasifica y crea reglas por bloque.
- **CU-06 Emparejar traslado/overnight:** el sistema propone el par débito↔crédito entre cuentas; el usuario confirma.
- **CU-07 Cuadrar por concepto:** el usuario ve totales por concepto y navega al detalle.
- **CU-08 Exportar reporte a equipo:** el usuario genera el archivo por período (p. ej. Asociaciones) con formato definido.
- **CU-09 Ver dashboard del mes:** ingresos vs gastos, EBITDA, comparativos, calendario.
- **CU-10 Proyectar cierre de mes:** el usuario configura método (histórico, promedio, coincidencia) y meta de crecimiento.
- **CU-11 Importar históricos:** carga masiva respetando la clasificación del formato de origen.

## 6. Historias de usuario

- Como **auxiliar**, quiero que al subir el Excel el sistema reconozca el banco por el IBAN, para no cargar mal la cuenta ni duplicarla.
- Como **auxiliar**, quiero ver los duplicados detectados y decidir incluir/excluir, para que el 100% de los movimientos reales quede cargado sin duplicidad indebida.
- Como **supervisor**, quiero crear una regla por bloque desde varios movimientos parecidos, para clasificar más rápido y que el motor aprenda.
- Como **director financiero**, quiero congelar el TC del mes al cierre y que se aplique a todos los movimientos USD, para conservar la integridad histórica.
- Como **director financiero**, quiero proyectar cuánto entrará del 28 al 30 por línea de ingreso, para ajustar la estrategia de caja.
- Como **auditor**, quiero ver quién clasificó o reclasificó cada movimiento y cuándo, para garantizar trazabilidad.

## 7. Reglas de negocio

### 7.1 Importación e identificación de banco
- **RN-01** El importador detecta la fila de inicio de movimientos y descarta residuos/encabezados del archivo.
- **RN-02** Si el estado de cuenta contiene un IBAN, se guarda la asociación `IBAN → banco → cuenta` en memoria. En cargas futuras, ese IBAN identifica el banco automáticamente y evita crear cuentas duplicadas.
- **RN-03** El importador asimila formatos internos con caracteres especiales propios de cada banco de CR [T1].
- **RN-04** Discrimina cuentas en CRC y en USD.
- **RN-05** Campos mínimos por movimiento: **Fecha, Documento, Débito, Crédito, Descripción, Banco, Concepto, Clasificación** (los dos últimos se completan por el motor o en Revisar).

### 7.2 Detección de duplicados (regla confirmada)
- **RN-06** Los archivos de origen se guardan **sin modificación** y se les calcula una huella (`source_file_hash`).
- **RN-07** **Duplicado real:** si una línea aparece duplicada **dentro del mismo documento de origen**, es real (caso BAC). El sistema **alerta** y permite decidir **incluir/excluir** movimiento a movimiento; el objetivo es que el **100% de los movimientos del banco quede incluido**.
- **RN-08** **Reimportación:** clave natural = `hash(cuenta_bancaria_id, fecha, débito, crédito, documento, índice_de_ocurrencia_en_archivo)`. Si la clave ya existe, el movimiento **no se reinserta** (idempotencia). La descripción **no** entra en la clave (algunos bancos la cambian según la descarga).
- **RN-09** El `índice_de_ocurrencia` distingue duplicados legítimos (1, 2, …) dentro de un mismo archivo, para que ambos se conserven cuando corresponde.

### 7.3 Motor de tipo de cambio (regla confirmada)
- **RN-10** Se registran tres cotizaciones por mes: **día 1, día 15 y último día** (auto-sync BCCR, con override manual).
- **RN-11** Durante el mes el TC es **provisional** y escalonado: días 1–14 usan el valor del día 1; a partir del 15 usan el promedio(día 1, día 15).
- **RN-12** **Al último día del mes se congela** el TC definitivo = **promedio(día 1, día 15, último día)** y se aplica **retroactivamente a la totalidad de los movimientos USD de ese mes**.
- **RN-13** El TC congelado es **inmutable** por empresa/mes; se almacena para siempre. Cada mes repite el ejercicio de forma independiente.
- **RN-14** Cada movimiento USD conserva `monto_original_usd` y `monto_crc` (recalculado al congelar).

### 7.4 Motor de clasificación
- **RN-15** El motor clasifica por **reglas y palabras clave editables**, que saben si aplican solo a **débito**, solo a **crédito** o son **mixtas**.
- **RN-16** **Umbral de confianza ≥ 90%** → asigna Concepto + Clasificación. Por debajo → estado **"No identificado"** (nunca clasifica dudoso).
- **RN-17** El motor **aprende** de las correcciones en Revisar; el diccionario `palabra_clave → (concepto, clasificación)` es **exportable e importable**.
- **RN-18** Concepto y Clasificación están **relacionados intrínsecamente** (alimentan la salud financiera y los estados). Se pueden crear nuevos (crecen/decrecen según la empresa).

### 7.5 Traslados de Fondos y Overnight
- **RN-19** Un traslado/overnight es un **débito en un banco ↔ crédito en otro** del mismo grupo. El motor propone el par por monto y fecha.
- **RN-20** En USD puede haber **diferencia menor por diferencial bancario**; se aplica una **tolerancia configurable** (monto absoluto o %). El neto de un par emparejado **no infla ingresos ni gastos** (§13, EBITDA).

### 7.6 Cuadre e integridad
- **RN-21** El Cuadre agrupa por Concepto el total de créditos y débitos y permite navegar al detalle (drill-down).
- **RN-22** Un movimiento mal clasificado corrompe el reporte de accionistas; por eso todo "No identificado" debe resolverse antes del cierre (alerta bloqueante configurable).

## 8. Excepciones

- Archivo de banco con formato desconocido → importación en cuarentena + solicitud de mapeo [T1].
- IBAN nuevo no asociado → el sistema pregunta a qué banco/cuenta pertenece y lo memoriza.
- Movimiento USD en mes aún no congelado → se muestra con TC **provisional** y bandera "sujeto a congelamiento".
- Duplicado ambiguo (aparece en archivo nuevo pero coincide con clave existente) → alerta con comparación lado a lado.
- Par de traslado sin contraparte (banco no cargado aún) → queda "pendiente de emparejar" y se resuelve al cargar el otro banco.
- Reclasificación posterior al cierre → permitida solo con rol autorizado y traza de auditoría (§26).

## 9. Riesgos

| Riesgo | Impacto | Mitigación |
|---|---|---|
| Duplicidad silenciosa | Estados inflados | RN-06 a RN-09 + alerta incluir/excluir |
| Mala identificación de banco | Datos cruzados | Memoria IBAN (RN-02) + confirmación |
| TC mal congelado | Distorsión histórica | Inmutabilidad + auditoría del congelamiento |
| Clasificación errónea | Reporte de accionistas erróneo | Umbral 90% + "No identificado" + cuadre bloqueante |
| Cambio de formato del banco | Importación rota | Adaptadores versionados + cuarentena |
| Traslados no netados | EBITDA inflado | Emparejamiento con tolerancia (RN-19/20) |

## 10. Dependencias

- **BCCR** (tipo de cambio de referencia) — §23.
- **Catálogo por empresa** (Concepto/Clasificación/Bancos) — prerequisito de clasificación.
- **CxP** — consume la "huella" de pagos para conciliar contra movimientos (contrato en §23).
- **Presupuesto** — consume totales por Concepto/Clasificación y período.
- **Motor de roles/permisos** transversal.

## 11. Flujo funcional

```
[Descarga banco] → [Importador: detecta IBAN/banco + inicio + moneda]
      → [Previsualización + marca duplicados (incluir/excluir)]
      → [Confirmar import] → [Motor clasifica ≥90%] → [No identificados → Revisar]
      → [Emparejar traslados/overnight] → [Movimientos (hoja de trabajo)]
      → [Cuadre por concepto] → {Dashboard, Proyecciones, Exportar, Reporte accionistas}
```

## 12. Wireframe descriptivo

- **Layout general:** barra superior con selector de **empresa** y **período**; menú lateral con las 8 pantallas; encabezado con totales vivos (créditos, débitos, diferencia) según filtros.
- **Importador:** zona de arrastre de archivo → tabla de previsualización con columnas normalizadas, chips de estado (Nuevo / Duplicado real / Reimportación), banda de resumen (líneas leídas, incluidas, excluidas, no identificadas).
- **Catálogo:** pestañas Bancos · Concepto · Clasificación · Motor (reglas y palabras clave con selector débito/crédito/mixto).
- **Revisar:** panel de filtros a la izquierda (fecha, período, monto, débito/crédito, patrón); tabla central seleccionable; acción "crear regla por bloque".
- **Movimientos:** tabla maestra con todos los filtros y ordenamientos; encabezado con total débitos, total créditos y diferencia según filtro.
- **Cuadre:** tabla por Concepto (crédito/débito), fila expandible a movimientos.
- **Dashboard:** tarjetas (Ingresos, Gastos, EBITDA, vs mes anterior) + calendario de ingresos y de gastos por día.
- **Proyecciones:** panel de configuración de método + meta de crecimiento; gráfico de senda de cierre por línea de ingreso.

## 13. Dashboard

- Por defecto muestra el **mes actual**; permite filtrar por otros períodos o "todos".
- **Ingresos vs Gastos**, **EBITDA del mes** [T2], **comparativo vs mes anterior** (absoluto y %).
- **Calendario de ingresos:** cantidad de transacciones e ingreso por día; **calendario de gastos** análogo.
- Los **traslados/overnight emparejados se excluyen** del cálculo de ingresos/gastos/EBITDA.

> [T2] Perímetro de EBITDA pendiente: qué Conceptos se consideran operativos y cuáles se excluyen (traslados, overnight, movimientos financieros, extraordinarios).

## 14. Menús

Importador · Catálogo · Revisar · Movimientos · Cuadre · Exportar · Dashboard · Proyecciones. (Visibilidad por rol y empresa.)

## 15. Botones (principales por pantalla)

- **Importador:** Subir archivo · Previsualizar · Incluir/Excluir duplicado · Confirmar importación · Importar históricos · Ver duplicados/inconsistencias · Limpiar duplicados.
- **Catálogo:** Nuevo banco · Nuevo concepto · Nueva clasificación · Nueva regla · Exportar/Importar diccionario de palabras clave.
- **Revisar:** Aplicar filtro · Detectar patrón · Clasificar selección · Crear regla por bloque.
- **Movimientos:** Buscar · Ordenar · Exportar vista.
- **Cuadre:** Expandir concepto · Exportar cuadre.
- **Dashboard:** Cambiar período · Comparar.
- **Proyecciones:** Elegir método · Definir meta · Recalcular · Guardar escenario.

## 16. Acciones

Importar, previsualizar, confirmar, clasificar, reclasificar, crear/editar/eliminar regla y palabra clave, emparejar/desemparejar traslado, incluir/excluir duplicado, congelar TC (rol autorizado), exportar, guardar escenario de proyección.

## 17. Formularios

- **Regla de clasificación:** nombre, aplica a (débito/crédito/mixto), palabras clave, Concepto destino, Clasificación destino, prioridad, empresa.
- **Banco/Cuenta:** nombre banco, IBAN, moneda (CRC/USD), alias, empresa.
- **Cotización TC:** fecha (1/15/último), valor, fuente (BCCR/manual).
- **Escenario de proyección:** método (histórico 2025 / promedio 2026 / coincidencia exacta), líneas de ingreso, rango de días, meta de crecimiento (%), empresa.

## 18. Campos (movimiento — modelo canónico)

`id`, `empresa_id`, `banco_id`, `cuenta_bancaria_id`, `fecha`, `documento`, `descripcion`, `debito`, `credito`, `moneda_original`, `monto_original`, `monto_crc`, `tc_aplicado`, `concepto_id`, `clasificacion_id`, `estado_clasificacion` (auto/revisado/no_identificado), `confianza`, `es_traslado`, `par_traslado_id`, `importacion_id`, `natural_key`, `indice_ocurrencia`, `incluido` (bool), `origen_historico` (bool), auditoría (§26).

## 19. Validaciones

- Débito y crédito no simultáneamente > 0 (salvo formato específico [T1]).
- Fecha dentro del período del estado de cuenta.
- Moneda USD ⇒ existe cotización del mes (o marca provisional).
- Clasificación ⇒ Concepto y Clasificación pertenecen al catálogo de la empresa activa.
- Confianza < 90% ⇒ `estado = no_identificado` (no se autoclasifica).
- No permitir cierre de período con movimientos "No identificado" pendientes (configurable).

## 20. Mensajes del sistema

- "Banco identificado por IBAN: BCR." / "IBAN nuevo: ¿a qué banco pertenece?"
- "Se detectaron N líneas duplicadas dentro del archivo. ¿Incluir o excluir?"
- "M movimientos ya existían (reimportación) y se omitieron."
- "TC de julio congelado: ₡X (promedio 1/15/último). Aplicado a K movimientos USD."
- "P movimientos quedaron como No identificado. Revisar antes del cierre."
- "Par de traslado propuesto: BCR (débito) ↔ BAC (crédito). ¿Confirmar?"

## 21. Estados

- **Importación:** cargada → previsualizada → confirmada → cerrada.
- **Movimiento:** no_identificado → clasificado_auto → revisado; y `incluido` true/false.
- **TC del mes:** provisional → congelado (inmutable).
- **Traslado:** pendiente_emparejar → emparejado.

## 22. Automatizaciones

- Identificación de banco por IBAN (memoria).
- Detección de inicio de movimientos y descarte de residuos.
- Conversión USD→CRC provisional/congelada.
- Clasificación ≥ 90% y aprendizaje continuo.
- Emparejamiento propuesto de traslados/overnight.
- Alerta de duplicados e inconsistencias.
- Sync automático de TC (BCCR) los días 1/15/último.
- Alertas de cuadre (no identificados, diferencias).

## 23. Integraciones

- **BCCR** → cotización de referencia (API; fallback manual). Frecuencia: 1/15/último.
- **CxP (contrato de "huella"):** al pagar por la macro se genera una **descripción única para el banco**; la terna **(descripción única + monto + fecha)** es la huella con la que, al importar el estado de cuenta, el movimiento se **empareja con el pago de CxP** y se clasifica en la cuenta que corresponde. Bancos expone un endpoint de conciliación que CxP consume.
- **Presupuesto** → Bancos publica totales por Concepto/Clasificación/período.

## 24. APIs (borrador REST · Go+Gin)

```
POST   /v1/empresas/{id}/bancos/importaciones            # subir archivo
GET    /v1/.../importaciones/{iid}/preview                # previsualización + duplicados
POST   /v1/.../importaciones/{iid}/confirmar
GET    /v1/.../movimientos?filtros...                     # hoja de trabajo
PATCH  /v1/.../movimientos/{mid}/clasificacion
POST   /v1/.../reglas                                     # crear regla / por bloque
GET/POST /v1/.../catalogo/{conceptos|clasificaciones|palabras-clave}
POST   /v1/.../palabras-clave/import  |  GET .../export   # diccionario
POST   /v1/.../traslados/emparejar
GET    /v1/.../tipo-cambio/{anio}/{mes}  | POST .../congelar
GET    /v1/.../cuadre?periodo=...
POST   /v1/.../exportaciones                              # reportes/equipos
GET    /v1/.../dashboard?periodo=...
POST   /v1/.../proyecciones                               # escenario
POST   /v1/.../conciliacion/match                         # contrato con CxP
```
Todos los endpoints filtran por `empresa_id` del contexto de sesión (tenant).

## 25. Modelo de datos (tablas núcleo)

`empresa`, `usuario`, `rol`, `usuario_empresa_rol` (multi-empresa), `banco`, `cuenta_bancaria` (IBAN, moneda), `tipo_cambio_cotizacion` (empresa, fecha, valor, fuente), `tipo_cambio_mes` (empresa, anio, mes, valor_congelado, estado), `importacion` (source_file_hash, estado), `movimiento_bancario` (§18), `concepto`, `clasificacion` (FK concepto; **campo opcional `cuenta_contable_futura`** para el mapeo NIIF posterior), `regla_clasificacion`, `palabra_clave`, `traslado_par`, `exportacion`, `proyeccion_escenario`, `auditoria_evento`.

- **Multi-tenant:** `empresa_id` en toda tabla transaccional; índices compuestos `(empresa_id, fecha)`.
- **Particionamiento** de `movimiento_bancario` por `(empresa_id, anio, mes)` para el volumen proyectado.
- **Preparación contable:** `clasificacion.cuenta_contable_futura` permite conectar con contabilidad/NIIF sin migrar datos.

## 26. Auditoría

Registro inmutable (append-only) de: importación (quién, archivo, hash), clasificación/reclasificación (valor anterior→nuevo, usuario, timestamp), congelamiento de TC, inclusión/exclusión de duplicados, emparejamientos, exportaciones. Retención mínima recomendada acorde a normativa fiscal CR. Trazabilidad completa por movimiento.

## 27. Seguridad

- Autenticación con login; **MFA** para roles con exportación/congelamiento.
- Autorización por rol × acción × empresa (RBAC dinámico); **aprobación mancomunada** disponible para acciones sensibles.
- Aislamiento estricto por `tenant_id` en cada consulta (defensa en profundidad, no solo a nivel de UI).
- Cifrado en tránsito (TLS) y en reposo; sanitización de archivos importados (los estados de cuenta de terceros son no confiables).
- Gestión de sesiones con expiración e invalidación.

## 28. KPIs

- % de movimientos autoclasificados (meta ≥ 90%).
- % de "No identificado" al cierre (meta → 0).
- Tiempo medio de importación por archivo.
- Ingresos vs Gastos y **EBITDA** [T2] del período.
- Diferencia de cuadre por concepto.
- Traslados emparejados vs pendientes.
- Precisión de proyección (proyectado vs real de cierre).

## 29. Reportes

Reporte de accionistas (por Concepto/Clasificación), cuadre por concepto, movimientos filtrados, evolución ingresos/gastos, comparativo mensual, calendario de ingresos/gastos, precisión de proyección. Todos con filtro por empresa y período.

## 30. Exportaciones

- Reportes en todos los formatos habituales (Excel, CSV, PDF).
- **Créditos por período para equipos** (hoy: Asociaciones, Depósito, Servicios de Emergencia — este último agrupa los bancos de Servicios de Emergencia y Depósito).
- Formato de equipo: `Fecha, Documento, Crédito, Descripción, Banco, Concepto, Clasificación, Consecutivo Largo`.
- **Consecutivo Largo (Davivienda):** cuando el banco es Davivienda, extraer de la descripción los 25 dígitos a partir de la posición 24 (equivalente a `=EXTRAE(desc;24;25)` → p. ej. `2026040712365478965874123`).

## 31. Notificaciones

Duplicados/inconsistencias detectados, no identificados pendientes al acercarse el cierre, TC congelado, traslados sin contraparte, fallo de sync BCCR, importación en cuarentena por formato nuevo. Canales: in-app y correo (configurable por rol).

## 32. Rendimiento esperado

- Volumen: 7–10k mov/mes/empresa, +25% anual → dimensionar a ~25–30k mov/mes a 3 años.
- Importación de un archivo mensual típico: previsualización < 5 s; confirmación < 10 s.
- Consultas de Movimientos con filtros: < 1 s hasta cientos de miles de filas (índices + particionamiento).
- Dashboard/Cuadre: < 2 s por período.

## 33. Escenarios de error

Archivo corrupto o vacío; formato de banco no reconocido; IBAN ilegible; USD sin cotización; colisión de clave natural; par de traslado inexistente; fallo de BCCR (usar último valor + bandera); intento de reclasificar período cerrado sin permiso; exportación con filtro vacío. Cada uno con mensaje claro (§20) y sin pérdida de datos.

## 34. Escalabilidad

- Alta de nuevas empresas sin cambios de esquema (multi-tenant por `empresa_id`).
- Nuevos bancos/formatos vía adaptadores versionados (patrón *strategy*), sin tocar el núcleo.
- Catálogos Concepto/Clasificación/reglas que crecen o decrecen por empresa.
- Particionamiento temporal y archivado de períodos antiguos.
- Motor de clasificación con diccionario portable entre empresas.

## 35. Mejoras futuras

- Clasificación asistida por ML con *feedback loop* explícito y explicabilidad (por qué se sugirió un concepto).
- Conexión directa a bancos vía Open Finance/API cuando esté disponible (reducir dependencia del Excel).
- Detección de anomalías/fraude en movimientos.
- Conciliación automática total con CxP y Nómina (huella extendida).
- Capa contable/NIIF sobre el mapeo `cuenta_contable_futura`.
- Proyecciones con estacionalidad y detección automática de meses extraordinarios.

---

## Revisión obligatoria del módulo (cierre del comité)

**¿Qué funcionalidades faltan / requieren aclaración?**
- [T1] Muestras de estado de cuenta por banco (para los adaptadores de formato y las reglas de descarte de residuos).
- [T2] Perímetro y fórmula de EBITDA (qué Conceptos son operativos).
- [T3] Matriz fina de roles × acción × empresa.
- Confirmar tolerancia por defecto para el diferencial en traslados USD (¿monto fijo o %?).
- Confirmar si el "cierre de período" es bloqueante (no permite avanzar con No identificados) o solo advierte.

**¿Qué procesos podrían automatizarse aún más?** Sync BCCR, emparejamiento de traslados por regla aprendida, sugerencia de reglas a partir de patrones recurrentes en Revisar.

**¿Qué riesgos existen?** Los de §9; el mayor es la duplicidad/mala clasificación que contamina el reporte de accionistas — mitigado con dedup, umbral 90% y cuadre bloqueante.

**¿Qué haría un ERP líder aquí?** Adaptadores de banco versionados, conciliación bancaria automática, diccionario de clasificación portable, y auditoría inmutable — todo contemplado.

**¿Qué funcionalidades premium podrían incorporarse?** Detección de anomalías, proyección con estacionalidad, conexión directa a bancos, y explicabilidad de la clasificación.

---
*Fin de la especificación funcional — Módulo Bancos v1.0. Sujeta a validación del Director Financiero antes de generar documentación técnica para Claude Code.*
