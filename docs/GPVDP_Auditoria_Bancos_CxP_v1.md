# Auditoría de integración Bancos ↔ CxP

**Fecha:** 16 de octubre de 2026
**Alcance:** sinergia, automatización y comunicación real entre los módulos Bancos y Cuentas por Pagar.
**Método:** lectura del código de las tres capas (handler / service / repository), de las migraciones y
del router; y medición contra la base de datos vigente. Los números de este informe son medidos, no
estimados.

---

## 1 · Veredicto

El puente entre los dos módulos **está bien diseñado y bien construido**. El problema no es cómo está
hecho: es **cuánto cubre**.

Hoy el puente une un solo tramo del ciclo —el pago que vuelve del banco reconocido por su huella— y
ese tramo está resuelto con un nivel de cuidado alto. Lo que falta es el resto del ciclo: los caminos
por los que el dinero se mueve sin pasar por ahí, y sobre todo **los controles que avisarían si el
puente dejara de funcionar**.

La medición más contundente:

| | |
|---|---|
| Movimientos bancarios cargados | 31.195 |
| Facturas en CxP | 4.542 |
| **Movimientos enlazados a una factura o remesa** | **0** |
| **Facturas en estado CONCILIADO** | **0** |
| Débitos de 2026 sin trazabilidad a una factura | **₡2.119.530.110** (5.105 movimientos) |

El puente existe, está probado unitariamente y corre solo al confirmar cada importación. **Nunca ha
transportado un pago.** Y —este es el punto de auditoría— el sistema no tiene cómo distinguir
«todavía no se ha pagado nada por acá» de «el reconocimiento se rompió». Son indistinguibles desde
afuera, que es exactamente la condición en la que un control falla sin que nadie se entere.

---

## 2 · Lo que está bien construido

Corresponde dejarlo asentado, porque es el estándar contra el que se mide lo que falta.

**El diseño del puente es el correcto.** Bancos es dueño del movimiento, CxP es dueño de la huella y
del estado del documento, y se hablan por un **puerto** (`ConciliadorCxP`), no por acoplamiento
directo. Sin el puerto inyectado el barrido simplemente no hace nada, en vez de romperse.

**La verificación del monto es una decisión de auditor, no de programador.** Si el banco debitó una
cifra distinta a la que la factura mandaba pagar, el sistema **no concilia** y lo reporta como
hallazgo con las dos cifras. Conciliar «casi» es la puerta por la que entran los descuadres.

**El pago agrupado (remesa) se enlaza contra la remesa, no contra una de sus facturas.** Elegir una
al azar diría que esa factura costó lo que costó el grupo entero. El código lo evita explícitamente.

**El enlace está protegido en dos de los tres caminos que podrían romperlo:**

- Dividir un movimiento enlazado: **bloqueado** (`ErrDivisionConCxP`).
- Revertir una carga con pagos conciliados: **bloqueado**, y el mensaje dice cuántas facturas hay en
  el medio.
- Marcarlo como pago externo: **excluido** del universo elegible.

**El par (concepto, clasificación) se deriva, no se copia.** `movimiento_bancario` tiene FK compuesta
y el documento podría traer un par inconsistente.

**La carrera está resuelta de verdad.** `FOR UPDATE` sobre la fila, las dos columnas de enlace
verificadas libres, y `enlazado=false` tratado como carrera y no como error.

**El guion opcional en la huella.** El patrón exigía `CXP-` y el canal del banco borra el guion: el
barrido no enganchó un solo movimiento en toda la vida del sistema y nadie se enteró. Está corregido
y documentado con la evidencia del débito SINPE real. Es el precedente directo del hallazgo 3.1.

---

## 3 · Hallazgos

### 3.1 · CRÍTICO — No existe el control de «pagos declarados que el banco nunca debitó»

**Evidencia medida:** 66 facturas en estado PAGADO, con fecha de pago programada entre el 10 de julio
y el 2 de octubre de 2026. Las 66 pasaron por `PAGAR_DOCUMENTO` y las 66 están asignadas a un lote.
El estado de cuenta cargado llega hasta el **14 de setiembre de 2026**. Ninguna está conciliada.

Para un auditor, la diferencia entre «lo que el sistema dice que pagamos» y «lo que el banco
efectivamente debitó» **es la partida conciliatoria de tesorería**. Es la primera cifra que se pide
en un arqueo. Hoy el sistema no la calcula, no la muestra y no la alerta.

Las dos lecturas posibles de esas 66 facturas son incompatibles entre sí, y el sistema no permite
distinguirlas:

- *El pago nunca se ejecutó* (la macro no se subió al banco, o el banco la rechazó en silencio) → hay
  66 facturas marcadas como pagadas que siguen debiéndose, y el proveedor lo sabe aunque nosotros no.
- *El pago se ejecutó y el reconocimiento falló* → hay un defecto activo en el barrido.

**Lo que falta:** un reporte de excepción que liste las facturas en PAGADO cuya fecha de pago ya quedó
cubierta por el estado de cuenta cargado y que no tienen movimiento bancario enlazado. El dato para
calcularlo ya existe: `MAX(fecha)` de los movimientos incluidos se consulta hoy en
`repository_responsabilidad.go:557` para otra cosa.

**Severidad crítica** porque no tiene detección: el modo de falla es silencioso y acumulativo.

---

### 3.2 · ALTO — Excluir un movimiento no respeta el enlace a CxP · **CERRADO 16-oct-2026**

`incluido` tiene tres escritores. Dos protegen el enlace; el tercero no.

| Camino | ¿Verifica el enlace? |
|---|---|
| Revertir la importación entera | **Sí** — bloquea y dice cuántas facturas hay en el medio |
| Dividir el movimiento | **Sí** — `ErrDivisionConCxP` |
| **Excluir el movimiento suelto** (mig 0093) | **No** |

Ni `service_excluir.go` ni `repository_excluir.go` consultan `documento_cxp_id` ni `remesa_id`. El
`UPDATE` solo exige `AND incluido`.

El daño está descrito textualmente en el código de la reversa, que por eso lo bloquea:

> «Excluirlos deja a la factura conciliada contra plata que ya no está en los libros, y el barrido de
> huellas no puede volver a mirarla porque filtra por `incluido`: el enlace queda huérfano para
> siempre.»

Verificado: `MovimientosConHuella` filtra `AND m.incluido AND m.documento_cxp_id IS NULL`. Un
movimiento excluido después de enlazado no vuelve a ser candidato por **ninguna** de las dos
condiciones. La factura queda CONCILIADA contra un débito que ya no existe en los libros, de forma
irreversible desde la aplicación.

Es el mismo riesgo que la reversa ya reconoció y cerró, por una puerta que se abrió después.

---

### 3.3 · ALTO — Un lote de pago nunca se cierra

**Evidencia medida:** 36 lotes, **todos en estado ABIERTO**, creados entre el 13 de julio y el 13 de
octubre de 2026.

La tabla `lote_pago` tiene `CHECK (estado IN ('ABIERTO','CERRADO'))`. Ningún punto del código escribe
`'CERRADO'` (verificado por búsqueda en todo `internal/`). Es una columna con restricción, con dos
valores posibles y **un solo escritor que siempre escribe el mismo**.

Consecuencias de proceso:

- **No hay corte.** Un lote cuya macro ya se subió al banco puede seguir recibiendo facturas. Lo que
  se pagó y lo que el lote dice que contiene pueden divergir sin que nada lo impida.
- **No hay punto de control.** El cierre del lote es el momento natural para exigir que todas sus
  facturas estén conciliadas, y es el disparador natural del hallazgo 3.1.
- El reporte del lote y la macro describen un conjunto que todavía puede cambiar.

---

### 3.4 · ALTO — El tablero no distingue pagado de conciliado · **CERRADO 16-oct-2026**

`repository_dashboard.go:84` agrupa los dos estados:

```go
const estadosCerrados = `('PAGADO', 'CONCILIADO', 'DENEGADO', 'ANULADO', 'LIQUIDADA')`
```

Y las consultas de las líneas 331 y 343 los tratan igual (`estado IN ('PAGADO','CONCILIADO')`).

Para la cartera es defendible: las dos salieron. Para la **tesorería** no: la diferencia entre ambas
es precisamente la plata que declaramos haber pagado y que el banco todavía no confirmó. El Director
Financiero mirando el tablero hoy no tiene forma de ver esa cifra, que es la que el hallazgo 3.1
dice que falta.

---

### 3.5 · MEDIO — El pago parcial no tiene salida

La conciliación exige coincidencia **exacta** («en colones no hay diferencial que justifique
tolerancia»), lo cual es correcto como criterio. El problema es lo que pasa después: el veredicto
`MONTO_DIFERENTE` **es terminal**. Se cuenta en el reporte del barrido y no hay ninguna pantalla ni
endpoint para resolverlo.

El propio código enumera las causas legítimas —«pago parcial, retención no aplicada, otra factura»—
y las deja sin camino. Cada una de ellas deja el movimiento sin enlazar y la factura sin conciliar,
para siempre, a menos que alguien las concilie a mano por `POST /cxp/conciliacion/match`, que no sabe
de montos parciales.

---

### 3.6 · MEDIO — No existen las notas de crédito del proveedor

El neto a pagar es `total − retención − anticipos`. No hay tabla, estado ni campo para una nota de
crédito emitida por el proveedor.

Es un hueco de dominio con efecto en cadena: el proveedor emite una NC, el neto del sistema queda
inflado, el banco debita menos, el barrido devuelve `MONTO_DIFERENTE`, y por 3.5 eso no tiene salida.
Una NC se vuelve una factura que no se puede conciliar nunca.

---

### 3.7 · MEDIO — El rebote del banco es enteramente manual

`REBOTADA` (el banco rechazó el pago dentro de un lote) solo se escribe desde
`POST /cxp/documentos/:id/rebotar`. **Evidencia:** 16 rebotes registrados, los 16 a mano.

Un rebote es detectable sin intervención: factura PROGRAMADA o PAGADA, en un lote cuya fecha de corte
ya está cubierta por el estado de cuenta cargado, sin débito enlazado. Es el mismo cálculo del
hallazgo 3.1 — un solo reporte resuelve los dos.

---

### 3.8 · MEDIO — Los anticipos en dólares no se restan del neto

Autodocumentado en `repository_conciliacion.go:25`:

```sql
COALESCE((SELECT SUM(aa.monto_crc) FROM anticipo_aplicacion aa
          WHERE aa.factura_id = d.id AND aa.activo AND d.moneda = 'CRC'), 0)
```

El `AND d.moneda = 'CRC'` evita el error peor (restar colones a una factura en dólares), pero deja
otro: **una factura en dólares con anticipo aplicado se paga completa**, ignorando el anticipo. El
comentario lo reconoce y nombra la condición de salida.

Hoy inerte —las 4.542 facturas están en CRC— pero la migración 0095 ya introdujo macros por moneda,
o sea que el sistema espera facturas en dólares.

---

### 3.9 · LATENTE — La huella no compara monedas · **CERRADO 16-oct-2026**

`ConciliarHuella` compara `m.debito` (en la moneda de la cuenta bancaria) contra `NetoAPagar` (en la
moneda del documento) **sin verificar que sean la misma moneda**. No hay una sola mención a `moneda`
en todo el camino de la conciliación por huella del lado de Bancos.

Hoy inerte por la misma razón que 3.8. Cuando haya facturas en dólares produce dos fallas opuestas:

- Factura USD pagada desde cuenta en colones → `MONTO_DIFERENTE` siempre (ruido permanente).
- Una factura de USD 1.500 y un débito de ₡1.500 → **concilian**, y es un error grave que ningún
  control detecta.

El arreglo es barato hoy (una comparación de moneda antes de comparar importes) y caro después.

---

## 4 · Puentes que no existen

No son defectos de lo construido: son tramos del ciclo que todavía no se abordaron.

### 4.1 · Banco → CxP: el gasto que sale sin factura

**Medido: ₡2.119.530.110 en débitos de 2026, ninguno trazable a una factura.**

Parte de eso es legítimamente ajeno a CxP (traslados, planilla, impuestos). Pero hoy **no hay forma de
separar** lo que debía tener factura de lo que no. El calendario de responsabilidades (mig 0082)
resolvió el caso de las obligaciones recurrentes —los ≈₡28 M/mes de Hacienda, CCSS y servicios— pero
no el gasto suelto: un débito a un proveedor conocido, por un monto de factura, que nadie registró.

Es la contrapartida exacta del hallazgo 3.1. Juntos forman el cuadre de dos columnas que una
organización grande exige todos los meses:

```
facturas sin débito   ←→   débitos sin factura
```

### 4.2 · Proyección de caja

CxP sabe qué vence y cuánto (cartera con aging, prioridad AA). Bancos sabe el saldo y tiene
proyecciones. **Nadie los cruza.** «¿Alcanza el saldo del jueves para el corte del viernes?» es la
pregunta diaria de tesorería y hoy se contesta con dos pantallas y una calculadora.

Las dos mitades ya están construidas. Falta solamente la resta.

### 4.3 · Nómina → Banco

Verificado: el módulo `nomina` no tiene ninguna referencia a Bancos. La planilla sale del banco todos
los meses y **no se concilia contra nada**. Es el segundo egreso de la organización.

Queda señalado como alcance no auditado hoy, no como hallazgo: puede ser una decisión deliberada.

---

## 5 · Orden de cierre propuesto

Ordenado por relación entre lo que protege y lo que cuesta.

| # | Qué | Cierra | Esfuerzo |
|---|---|---|---|
| 1 | Guarda de enlace en `ExcluirMovimiento` | 3.2 | Mínimo — es la guarda que dividir ya tiene |
| 2 | Comparar moneda antes que importe en la huella | 3.9 | Mínimo — y caro si se posterga |
| 3 | Reporte de excepción de dos columnas: facturas sin débito ↔ débitos sin factura | **3.1**, 3.7, 4.1 | Medio — el dato ya existe |
| 4 | Separar PAGADO de CONCILIADO en el tablero | 3.4 | Bajo |
| 5 | Cierre de lote, con los pendientes de conciliación a la vista | 3.3 | Medio |
| 6 | Resolución del `MONTO_DIFERENTE` (pago parcial / diferencia justificada) | 3.5 | Medio |
| 7 | Notas de crédito del proveedor | 3.6, parte de 3.5 | Alto — es dominio nuevo |
| 8 | Anticipos en moneda extranjera | 3.8 | Medio |
| 9 | Proyección de caja CxP + Bancos | 4.2 | Medio — las dos mitades existen |

Los puntos **1, 2 y 4** son correcciones de cirugía: tocan pocas líneas y cierran riesgos reales.
El punto **3 es el de mayor retorno de todo el informe**: con un solo reporte se cierra el hallazgo
crítico, se automatiza la detección de rebotes y se cubre el lado que hoy no tiene ningún control.

---

## 6 · Correcciones aplicadas · 16 de octubre de 2026

Se cerraron los tres puntos de cirugía del plan (1, 2 y 4). Nada de esto cambia el comportamiento
del camino feliz: son guardas que cierran puertas por las que se perdía información.

### 6.1 · Guarda del enlace al excluir (hallazgo 3.2)

`ExcluirMovimiento` exige ahora `documento_cxp_id IS NULL AND remesa_id IS NULL` **en el WHERE del
UPDATE**, no en una lectura previa: entre leer y escribir, el barrido de huellas puede enlazar el
movimiento, y la comprobación habría mirado un estado que ya no es.

Cuando el UPDATE no afecta filas se diagnostica el porqué, porque el arreglo es distinto: un
movimiento enlazado se desenlaza de CxP (`ErrExclusionConCxP`, 422, mismo trato que
`ErrDivisionConCxP`), y uno ya excluido no necesita nada (`ErrMovimientoNoExcluible`, 409, el
comportamiento anterior).

Pruebas: dos contra PostgreSQL real —factura suelta y pago agrupado, porque son columnas distintas
y una guarda que mirara solo una dejaría pasar justo los pagos que cierran cinco facturas— más la
traducción HTTP. Falsificadas por mutación: quitar la guarda, quitar solo la mitad de la remesa, y
quitar el `case` del handler. Las tres ponen las pruebas en rojo.

### 6.2 · La moneda se compara antes que el importe (hallazgo 3.9)

El puerto `ConciliadorCxP.ConciliarHuella` recibe ahora la moneda junto con el monto. Bancos la
toma de `cuenta_bancaria.moneda` (NOT NULL, CHECK CRC|USD) y CxP la compara contra la de la factura
—o contra CRC en una remesa, que se declara siempre en colones— **antes** de mirar los importes.

Cuando difieren no se concilia y se devuelve el veredicto nuevo `MONEDA_DIFERENTE`, con las dos
monedas en el detalle. **No se convierte**: con qué tipo de cambio —el del día del débito, compra o
venta— es una regla que no está definida, y elegir una acá la habría inventado. Queda como decisión
pendiente del Director Financiero.

Pruebas: en CxP, que ₡1.500 no cierran una factura de USD 1.500 y que el estado del documento no se
toca; que minúsculas y espacios no son otra moneda; que la moneda manda sobre el importe cuando los
dos difieren; y que una remesa en colones no la paga un débito en dólares (donde el daño es mayor:
cierra todas sus facturas de una vez). En Bancos, que el barrido propaga la moneda de la cuenta.
Falsificadas por mutación: quitar la guarda de la factura, quitar la de la remesa, y dejar de
mandar la moneda desde Bancos.

### 6.3 · La partida conciliatoria en el tablero (hallazgo 3.4)

`CarteraCxP` tiene dos campos nuevos: `PorConfirmar` (cantidad y monto de las facturas en PAGADO
que ningún movimiento bancario confirmó) y `DiasSinConfirmar` (la antigüedad del pago declarado más
viejo, medida desde el EVENTO de auditoría y no desde `actualizado_en`, que se reescribe con
cualquier edición posterior).

Va en consulta aparte de `carteraCxP` porque no sale del CTE `abiertos`: PAGADO ya no está en la
cartera viva, y eso es exactamente lo que lo hacía invisible. `estadosCerrados` y las consultas de
«pagadas del período» **no se tocaron**: ahí agrupar PAGADO y CONCILIADO es correcto, las dos
salieron. Lo que faltaba no era cambiar el cálculo sino exhibir la diferencia.

En pantalla es una banda propia bajo los KPI de cartera —no entre ellos, porque no es cartera—
visible también en cero: un control que solo aparece cuando falla no se distingue de un control que
no existe.

**Medido en vivo tras el cambio: ₡3.023.476,84 en 66 facturas, la más vieja con 96 días.** Es el
hallazgo 3.1 hecho visible por primera vez.

Pruebas: cuatro contra PostgreSQL real (separa PAGADO de CONCILIADO; la cifra baja a cero al
conciliar; una pagada sin evento suma al monto pero no envejece; aislamiento por empresa).
Falsificadas por mutación: contar también las CONCILIADAS —que reproduce el defecto original y da
₡10.500.000 en vez de ₡1.500.000—, tomar el pago más nuevo en vez del más viejo, y quitar el filtro
de empresa.

### 6.4 · Lo que estas correcciones NO cierran

El hallazgo **3.1 sigue abierto**. El tablero ahora *muestra* la partida conciliatoria, que es la
mitad del problema; la otra mitad —el reporte de excepción que la explica factura por factura y la
cruza contra los débitos sin factura— es el punto 3 del plan y no se hizo.

Dicho de otro modo: hoy se puede *ver* que hay ₡3 M sin confirmar desde hace 96 días. Todavía no se
puede *saber* cuáles ni por qué.

### 6.5 · Hallazgos reportados por el Director Financiero · mig 0102

Tres defectos señalados sobre la pantalla, el mismo día. Dos eran reales y uno ya estaba resuelto a
medias.

**La factura de contado ya pagada solo se podía ANULAR.** La causa era una asimetría de
transiciones: `Anular` llegaba hasta PROGRAMADO y `Liquidar` se quedaba en REVISADO, así que en la
pestaña «Por pagar» —donde aparece el candado— la única salida ofrecida era la deshonesta. Anular
dice «esta factura no existió» cuando el dinero salió de verdad.

Ahora hay cierre con medio declarado (caja chica, viáticos, tarjeta, efectivo), desde los **mismos
estados desde los que se puede anular** —si el cierre honesto no llegara igual de lejos que el
deshonesto, se seguiría anulando—, con motivo obligatorio de 20 caracteres y vale del fondo
opcional. Queda en **LIQUIDADA y no en PAGADO**: en PAGADO entraría en la partida conciliatoria del
§6.3 y nunca conciliaría, inflándola para siempre. Si la factura estaba en un corte, sale de él y
la respuesta lo dice.

**El total del corte y del lote se mostraba siempre en colones.** Confirmado: un corte de USD
384,20 se anunciaba como «₡174 015,71» mientras la fila de abajo, en la misma pantalla, decía «USD
384,20». Ahora el titular va en la moneda en que se va a pagar, con el equivalente al lado, y un
corte mixto se muestra separado por moneda en vez de sumar peras con manzanas.

**La moneda en el nombre de la macro ya estaba** —el archivo bajaba como `macro-pagos-CRC.txt`—,
pero faltaba el número del lote. `NombreMacro` existía desde la 0095, lo contemplaba y nadie la
llamaba: dos cortes del mismo día bajaban con el mismo nombre y el navegador los dejaba como «(1)»
y «(2)». Verificado en vivo: ahora baja como **`macro-lote-0025-CRC.txt`**.

Un dato que salió de revisar esto: **ya hay facturas en dólares en operación**, así que el hallazgo
3.9 no era latente como se estimó al escribir este informe. El arreglo llegó el mismo día.

### 6.6 · Lo revertido se veía como plata viva

Reportado el mismo día: tras revertir una carga duplicada, el encabezado decía «2 movimientos ·
créditos ₡0,00» sobre una lista de **dos filas de ₡75 000 cada una**; y al recargar, «3 movimientos
· ₡75 000» sobre **tres filas de ₡75 000**.

Ninguno de los dos números estaba mal. El conteo cuenta todo lo que la lista muestra —revertir no
borra, y lo que desaparece de la pantalla no se revisa más— y los totales de plata no suman lo que
está fuera de los libros. **Lo que faltaba era decirlo.**

El backend ya devolvía `excluidos` en el resumen, y el comentario de ese campo lo advertía con
todas las letras: *«sin mostrarlo, los dos números se contradicen en silencio»*. La pantalla nunca
lo leyó — el tipo del cliente ni siquiera lo declaraba. Es el patrón conocido del proyecto: un dato
con escritor y sin lector.

Peor: en la lista, la **única** diferencia visible entre una fila revertida y una viva era que a la
revertida no le salía el botón «⋯». Nadie puede interpretar eso.

Ahora el encabezado dice «N fuera de los libros — se ven, no suman» y la fila va tachada con la
etiqueta, igual que «Mi partida», que ya lo hacía bien desde que el mismo problema se detectó ahí.
Dos pantallas que muestran lo mismo tienen que decirlo igual.

**Y la alineación**, señalada sobre «Mi partida»: la etiqueta iba al lado del monto, lo que corría
el número a la izquierda y rompía la columna. Comparar montos en diagonal —que es como se revisa
una lista— se volvía imposible. Ahora la etiqueta va debajo y los números quedan cuadrados a la
derecha.

### 6.7 · Observación de método

El paquete `bancos` **no tiene** el centinela que lee el fuente y exige que cada error de dominio
tenga su traducción HTTP (`cxp/handler_centinelas_test.go` sí lo tiene). Es el modo de falla que ya
se repitió varias veces en este proyecto: un centinela nuevo sin su `case` sale como 500 «error
interno» y el mensaje redactado nunca llega a la pantalla, con toda la suite en verde.

En `ErrExclusionConCxP` se cubrió con una prueba explícita, pero el próximo error que alguien
agregue a `bancos` no tendrá quien lo vigile. Copiar ese centinela a `bancos` es barato y queda
recomendado.

---

## 7 · Nota sobre el método

Todas las cifras de este informe se midieron contra la base vigente el 16 de octubre de 2026. Las
afirmaciones sobre el código se verificaron leyendo las tres capas, no por búsqueda de texto.

Dos afirmaciones quedan explícitamente marcadas como **no determinables con la información
disponible**, y se reportan como tales y no como defectos:

- Si las 66 facturas en PAGADO corresponden a pagos reales no reconocidos o a ejercicios del flujo
  que nunca llegaron al banco. Esa indistinguibilidad **es** el hallazgo 3.1.
- Si la ausencia de conciliación Nómina ↔ Banco (4.3) es un hueco o una decisión tomada.
