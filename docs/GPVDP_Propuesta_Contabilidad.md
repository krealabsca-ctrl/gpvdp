# Propuesta formal · Módulo de Contabilidad para el ERP GPVDP

**Fecha:** 23 de octubre de 2026
**Para:** Dirección Financiera — Grupo Valle de Paz
**Base:** Diagnóstico de estructura y viabilidad del 23-oct-2026 (`GPVDP_Diagnostico_Contable.md`)
**Estado:** propuesta para aprobación. **No se ha escrito ninguna línea de código.**

---

## 1 · Objeto

Incorporar al ERP GPVDP una capa de contabilidad formal, multiempresa y auditable, que produzca
estados financieros trazables hasta el documento que los origina, **sin modificar ninguno de los
módulos que hoy están en operación**.

## 2 · Por qué esta propuesta es viable

El diagnóstico verificó siete condiciones estructurales. Tres son determinantes:

1. **El dinero nunca pasó por punto flotante.** `numeric(16,2)` en base, `decimal` en el servidor,
   texto en el transporte. Un sistema contable que guarda dinero en coma flotante no es auditable,
   y corregirlo después obliga a reescribirlo entero.
2. **Los datos del asiento ya se capturan.** La factura de proveedor trae subtotal, IVA,
   retención, total, moneda, proveedor, clasificación, departamento y clave de Hacienda.
3. **El enganche ya existe.** La tabla de clasificaciones tiene la columna `cuenta_contable_futura`
   desde el diseño original, hoy vacía.

A eso se suman multiempresa con aislamiento probado, dimensiones de departamento y sede,
auditoría append-only y trazabilidad hasta el archivo original del banco.

## 3 · Principio rector

> **La contabilidad lee; no modifica.**

Los asientos se generan **desde el documento** —factura, cargo de contrato, planilla— y se
escriben en tablas nuevas. El movimiento bancario deja de ser origen y pasa a ser **cancelación y
conciliación**.

Esto tiene dos consecuencias que son el centro de la propuesta:

- **Se contabiliza por devengo, no por caja.** Si los asientos salieran del banco, se reconocería
  el gasto cuando la plata se mueve, y eso no produce información bajo NIIF.
- **Ningún módulo en operación se toca.** Si la capa contable se apagara, el ERP seguiría
  funcionando exactamente igual.

---

## 4 · Alcance por fases

Cada fase tiene entregable, criterio de aceptación verificable y dependencia. **Una fase no se
declara terminada hasta que su criterio se cumple y se demuestra.**

### Fase 1 · Plan de cuentas en el sistema

| | |
|---|---|
| **Entregable** | Tabla de cuentas contables con jerarquía de cinco niveles, importador del Excel del Director Financiero, pantalla de consulta y mantenimiento |
| **Criterio de aceptación** | Las 258 cuentas del catálogo cargadas, la jerarquía navegable, y una prueba que impida crear una cuenta de movimiento colgando de otra de movimiento |
| **Depende de** | Decisión D1 (plan único o por empresa) y validación del contador |
| **Riesgo sobre lo existente** | **Nulo.** Tabla nueva que ningún módulo lee |
| **Tamaño** | Bajo |

### Fase 2 · Mapeo preclasificación → cuenta contable

| | |
|---|---|
| **Entregable** | Equivalencias por empresa, pantalla para construirlas, **reporte de cobertura** (qué clasificaciones en uso aún no tienen cuenta) y **detector de naturaleza cruzada** (clasificaciones cuyo concepto no coincide con la clase de su cuenta) |
| **Criterio de aceptación** | 100 % de las clasificaciones **con movimientos** mapeadas, o declaradas explícitamente como no contabilizables, con su motivo. Cero clasificaciones con naturaleza cruzada sin resolver |
| **Depende de** | Fase 1 · decisión del contador sobre cada equivalencia |
| **Riesgo sobre lo existente** | **Nulo.** Llena una columna hoy vacía |
| **Tamaño** | Bajo (el software). **El criterio contable es el que marca el ritmo** |

#### Por qué el mapeo es el trabajo real, y no el catálogo

El catálogo ya existe: está en Softland y en la reestructuración del Director Financiero. Cargarlo
al sistema es la parte barata. **Lo que no existe en ninguna parte es la equivalencia**, y no es
mecánica. Medido sobre el catálogo de Valle de Paz:

| | ERP (preclasificación) | Catálogo contable |
|---|---|---|
| Clasificaciones / cuentas «Caja Chica» | **23** | **15** |
| Bajo qué naturaleza | concepto **Gastos** | clase **1 · ACTIVO** (`1-01-01-001-xxx`) |

Tres cosas salen de ahí, y las tres son decisiones, no conversiones:

1. **La cantidad no calza.** Nueve clasificaciones del ERP no tienen cuenta (*Ciudad Neilly*,
   *Convenios*, *Legal*, *Logística*, *María Martinez*, *Memorial Pets*, *Mercadeo*,
   *PEAJES / QUICK PASS*, *Religiosa de Cartago*). Cada una es una decisión: ¿cuenta nueva,
   o van todas a una existente?
2. **La naturaleza está cruzada.** En el ERP la caja chica cuelga de *Gastos*; contablemente es
   **activo**. Reponer una caja chica no es un gasto: es mover plata de Banco a Caja Chica. El
   gasto nace al liquidar los vales, contra la cuenta que corresponda. **Mapear esas 23 a una
   cuenta de gasto infla el estado de resultados y deja la caja chica fuera del balance.**
3. **El dato fino ya existe en otro módulo.** El sistema lleva fondos y vales de caja chica con su
   gasto clasificado. La contabilidad debe tomarlo de ahí, no del movimiento bancario.

Por eso la Fase 2 entrega el **detector de naturaleza cruzada**: señala toda clasificación cuyo
concepto en el ERP no concuerda con la clase de la cuenta a la que se la mapea. Convierte un
problema invisible en una lista finita que el contador resuelve una vez.

**Esto no es un defecto de la preclasificación.** Responde otra pregunta —«¿a dónde se fue la
plata?»— y para control de gasto está bien como está. Lo que no se puede es asumir que esa
respuesta sirve tal cual como asiento contable.

### Fase 3 · Motor de asientos, diario y mayor

> **Actualización del 27-oct-2026 — la fase se partió en dos.** Al medir el sistema apareció un
> hueco que no estaba en esta propuesta: **faltaba la mitad de la partida doble**. La Fase 2 mapeó
> la clasificación de cada partida, pero `cuenta_bancaria` no tenía ninguna columna hacia
> `cuenta_contable`, así que de un movimiento del banco salía una sola pata. Eso hay que
> resolverlo igual, se contabilice por caja o por devengo.
>
> De ahí el corte:
>
> | | Qué | Estado |
> |---|---|---|
> | **3A** | Estructura del asiento con partida doble imposible de violar · enlace cuenta bancaria → cuenta contable · idempotencia por documento · libro diario y mayor | **Construida y verificada** (mig 0111) |
> | **3B** | El generador: de qué documento sale cada asiento y en qué dirección | **Bloqueada por A3** |
>
> 3A no depende de A3 y por eso se construyó: nada de lo que contiene cambia según lo que diga la
> matriz de reglas. 3B sigue sin arrancar, como recomendaba la sección 9.
>
> **Lo que 3A dejó medido y listo:** el plan contable trae el IBAN dentro del nombre de la cuenta,
> así que **5 de las 16 cuentas bancarias se enlazan solas y cubren el 79 % de los movimientos**
> (15.873 de 20.082). Las otras 11 son decisión manual.
>
> **Actualización del 29-oct-2026 — 3B CONSTRUIDA, por caja.** El Director Financiero cerró A3:
> *«no es cuando debo sino cuánto pagué, y no es cuánto me deben sino cuánto ingresó. Actualmente
> la lógica de la empresa es cuando se materializa porque el flujo se maneja bancario»*. El
> generador sale del movimiento bancario (mig 0112, pantalla `/conta/generar`).
>
> **Queda dicho con todas las letras: por caja NO cumple NIIF.** No es comparable con Softland ni
> sirve para un due diligence. El destino declarado es devengo y el modelo lo admite sin rehacer
> —`origen_tipo` acepta `CXP_FACTURA` y la idempotencia es por documento—, pero **lo registrado
> por caja no se reexpresa solo**: la fecha de corte de esa migración es la decisión **D3**, que
> sigue abierta.
>
> Verificado sobre Valle de Paz, agosto 2026: **6.340 asientos de 8.219 movimientos (77 %) en
> 4,2 segundos**, con el libro cuadrando exacto en ₡339.759.841 y cero asientos descuadrados.
> Regenerar no duplica, y revertir anula con motivo sin borrar.

| | |
|---|---|
| **Entregable** | Asiento y línea de asiento con **partida doble garantizada en la escritura**; generadores desde CxP, CxC y nómina; libro diario; mayor general |
| **Criterio de aceptación** | Imposible insertar un asiento descuadrado: la invariante se prueba intentando violarla y la base lo rechaza. Todo asiento apunta a su documento de origen. Un asiento generado dos veces desde el mismo documento no se duplica |
| **Depende de** | Fases 1 y 2 · matriz de reglas de contabilización aprobada |
| **Riesgo sobre lo existente** | **Bajo.** Solo lee los documentos; no los modifica |
| **Tamaño** | **Alto — el 60 % del esfuerzo total** |

### Fase 4 · Apertura y cierre contable

> **Actualización del 29-oct-2026 — CONSTRUIDA Y VERIFICADA** (mig 0113, pantalla
> `/conta/cierre`). Las tres piezas funcionan de punta a punta:
>
> - **El balance de comprobación** cuadra en los tres pares a la vez —saldo anterior, movimientos
>   del rango y saldo final—, y es lo que decide si un mes se puede cerrar.
> - **El cierre** lo hace cumplir un **disparador de base**, no el servicio: un mes cerrado
>   rechaza asientos nuevos, modificaciones, anulaciones y líneas sueltas, aunque el intento venga
>   de un `psql` a mano. Reabrir exige motivo y queda registrado en el expediente del período.
> - **La apertura** se carga desde el balance de comprobación del sistema anterior, en Excel.
>
> **Lo que todavía hace falta de la empresa es A2: los saldos iniciales reales y su fecha de
> corte.** El lector está probado con un archivo de muestra —reconoce «Código Cuenta» y «Saldo
> Deudor / Saldo Acreedor», lee `₡48.200.000` y `125.450.300,55`, descarta las cuentas en cero y
> deja fuera las que no existen en el plan, diciendo cuáles— pero **sin el archivo de verdad el
> balance muestra solo lo que registró el sistema, y el activo no es el activo**. La pantalla lo
> avisa en todas las consultas mientras falte.
>
> Verificado sobre Valle de Paz: apertura de 6 cuentas por **₡174.500.300,55** cuadrada, segunda
> carga rechazada, y el balance del mes siguiente arrancando desde ese saldo. Los datos de ensayo
> se borraron después; **el mapeo de partidas que se usó para probar era automático, no criterio
> contable, y por eso no quedó**.

| | |
|---|---|
| **Entregable** | Carga de saldos iniciales, balance de comprobación, período contable con cierre y reapertura con motivo |
| **Criterio de aceptación** | El balance de comprobación cuadra en cero. Un período cerrado rechaza asientos con fecha dentro de él. La reapertura exige motivo y queda registrada |
| **Depende de** | Fase 3 · **saldos iniciales provistos por la empresa** |
| **Riesgo sobre lo existente** | **Bajo** |
| **Tamaño** | Medio |

### Fase 5 · Estados financieros

| | |
|---|---|
| **Entregable** | Estado de situación financiera, estado de resultados, estado de flujos de efectivo y estado de cambios en el patrimonio |
| **Criterio de aceptación** | Cada cifra de cada estado se puede abrir hasta el asiento y desde el asiento hasta el documento. Activo = Pasivo + Patrimonio, demostrado sobre datos reales |
| **Depende de** | Fase 4 · formato aprobado por el contador |
| **Riesgo sobre lo existente** | **Nulo.** Solo lee |
| **Tamaño** | Medio-alto. El flujo de efectivo es la pieza difícil |

### Fase 6 · Consolidación y preparación para due diligence

| | |
|---|---|
| **Entregable** | Cuentas entre compañías, eliminaciones, separación de partes relacionadas, conciliación entre EBITDA y resultado contable, recurrente contra no recurrente |
| **Criterio de aceptación** | El consolidado del grupo cuadra y las operaciones entre empresas quedan en cero después de eliminar |
| **Depende de** | Fase 5 · política de eliminaciones aprobada |
| **Riesgo sobre lo existente** | **Nulo** |
| **Tamaño** | Medio |

---

## 5 · Lo que la empresa debe aportar

Sin esto, las fases no arrancan. **Ninguno depende de programación.**

| # | Qué | Bloquea | Por qué no lo puedo resolver yo |
|---|---|---|---|
| **A1** | **Contador colegiado que valide** catálogo, políticas de reconocimiento y tratamiento fiscal CR | Fase 1 en adelante | Son criterios profesionales, no decisiones técnicas |
| **A2** | **Saldos iniciales** y su fecha de corte, desde Softland o un balance certificado | Fase 4 | No se pueden inventar ni derivar |
| **A3** | **Matriz de reglas de contabilización** por tipo de transacción, aprobada | Fase 3 | Define qué cuenta se debita y cuál se acredita en cada caso |
| **A4** | **Decisión sobre el plan de cuentas**: único del grupo o uno por empresa | Fase 1 | Cambia el modelo de datos; corregirlo después es caro |
| **A5** | **Los mapeos**, clasificación por clasificación | Fase 2 | Es criterio contable sobre cada partida |
| **A6** | **Datos de producción** para medir el rezago real de clasificación | Planificación | Lo que hay hoy en la base es de prueba |

---

## 6 · Decisiones pendientes

| # | Decisión | Impacto si se decide tarde |
|---|---|---|
| **D1** | ¿Plan de cuentas único del grupo o uno por empresa? | **Alto.** Rehacer el modelo de la Fase 1 |
| **D2** | ¿La clase 5 «Compras» es costo de ventas? | Medio. Si lo es, exige inventario valorizado y costeo |
| **D3** | ¿Desde qué fecha arranca la contabilidad? | **Alto.** Define de qué corte salen los saldos iniciales |
| **D4** | ¿Se migra el histórico de Softland o se arranca en limpio desde una fecha? | **Alto.** Define si hay comparativos del año anterior |
| **D5** | ¿Activos fijos dentro de este alcance o en un proyecto aparte? | Medio. Sin ellos no hay depreciación ni balance completo |
| **D6** | ¿Las tres empresas a la vez o Valle de Paz primero? | Medio. Afecta la secuencia, no el diseño |

**Recomendación sobre D6:** empezar por **una sola empresa**. El diseño multiempresa ya está
resuelto; lo que se gana es validar el modelo contable completo sobre un caso real antes de
replicarlo.

---

## 7 · Riesgos

| Riesgo | Probabilidad | Impacto | Mitigación |
|---|---|---|---|
| El mapeo se detiene por falta de criterio contable | **Alta** | Alto | Fase 2 entrega reporte de cobertura; lo que falta se ve y se persigue |
| Saldos iniciales que no cuadran con Softland | Media | **Alto** | El balance de comprobación lo detecta antes de emitir un estado |
| Asientos duplicados al regenerar desde un documento | Media | Alto | Unicidad por documento de origen, verificada en la base |
| Se contabiliza por caja sin querer | Baja | **Crítico** | El motor **no acepta** el movimiento bancario como origen de un asiento de resultado |
| Que el alcance crezca hacia activos fijos e inventario | **Alta** | Medio | D5 lo deja explícitamente fuera hasta decidirlo |
| Romper algo en operación | **Baja** | Alto | Ninguna fase modifica tablas existentes salvo llenar una columna vacía |

---

## 8 · Lo que esta propuesta NO promete

Con todas las letras, para que no se interprete de más:

1. **No vuelve auditada a la empresa.** Un sistema que produce estados financieros no sustituye
   una auditoría. Solo un auditor puede emitir esa opinión.
2. **No sustituye al contador.** El sistema ejecuta reglas; no decide cuáles son.
3. **No garantiza cumplimiento fiscal.** Los criterios tributarios de Costa Rica los valida un
   profesional del país.
4. **No incluye activos fijos, depreciación ni inventario valorizado**, salvo decisión expresa en
   D5.
5. **No se comprometen fechas en este documento.** La velocidad depende de A1 a A6, que no están
   bajo mi control. Las fechas se fijan cuando esos insumos tengan responsable y plazo.
6. **No se declara ninguna fase terminada sin su criterio de aceptación demostrado.**

---

## 9 · Qué recomiendo aprobar hoy

**Fases 1 y 2.** Son de tamaño bajo, riesgo nulo sobre la operación, y se pueden construir
**mientras** avanzan las decisiones contables. Al terminarlas, el grupo tendrá:

- El plan de cuentas dentro del sistema, versionado y navegable.
- El mapeo entre lo que ya se clasifica a diario y la contabilidad.
- Un reporte de cobertura que muestra, en todo momento, cuánto falta para poder contabilizar.

Ese reporte es el que permite decidir con datos si seguir a la Fase 3, y cuándo.

**Lo que recomiendo NO aprobar todavía:** la Fase 3. No debería arrancar hasta que D1, D3 y A3
estén cerradas. Empezar el motor de asientos sin saber si el plan es único o por empresa, desde
qué fecha se contabiliza y con qué reglas, es construir algo que habrá que rehacer.

---

*Documento preparado sobre el sistema GPVDP en su migración 108. Es una propuesta técnica de
implementación; los criterios contables, fiscales y de presentación bajo NIIF deben ser validados
por un profesional competente en Costa Rica antes de construir.*
