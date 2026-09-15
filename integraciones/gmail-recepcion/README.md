# Conectar el buzón de una empresa al ERP

Esto reemplaza al extractor que volcaba los XML en una hoja de cálculo para después subir un `.xlsx`
a mano. Al terminar, **la factura entra sola**: llega al correo y aparece en CxP.

Hay que hacerlo **una vez por empresa**, porque cada una tiene su propio buzón y su propia credencial.

---

## Antes de empezar: dos cosas que hay que saber

**1. El script viejo hay que apagarlo, no dejarlo corriendo al lado.** Los dos usan la misma
etiqueta `Procesado-XML`, así que si el viejo sigue activo va a etiquetar hilos que el nuevo nunca
llegó a enviar, y esas facturas quedan invisibles. El paso 6 es borrar su disparador.

**2. Reusar esa etiqueta es a propósito y es una ventaja.** Todo el histórico ya está etiquetado por
el script viejo, así que el conector nuevo **no lo va a volver a mandar**: esas facturas ya están en
el ERP por la vía del Excel. El corte queda limpio, sin cargar nada dos veces.

---

## Paso 1 · Dar de alta el buzón en el ERP

En el ERP, con la empresa correcta seleccionada arriba:

**CxP › Buzones › Conectar un buzón**

| Campo | Qué poner |
|---|---|
| Nombre | Algo que se lea, p. ej. «Buzón de facturación» |
| Correo del buzón | La dirección **exacta** a la que llegan las facturas de esa empresa |

Al guardar aparece la **credencial**. Copiala ya: **no se puede volver a ver** —el sistema guarda
solo su huella—. Si se pierde, se rota y se pega la nueva.

> El correo que escribís acá **no es decorativo**: el conector reporta a qué buzón llegó cada correo
> y el ERP lo compara contra este. Si la credencial termina pegada en el buzón de otra empresa, el
> ERP lo detecta y lo dice en vez de meter las facturas donde no van.

## Paso 2 · Abrir el editor del script

En la hoja de cálculo donde vive hoy el extractor de esa cuenta:

**Extensiones › Apps Script**

Si preferís empezar limpio, creá una hoja nueva desde esa misma cuenta de Google y abrí su Apps
Script. La hoja solo se usa para la bitácora; el dato vive en el ERP.

## Paso 3 · Pegar el código

Borrá todo el contenido de `Código.gs` y pegá el de **`Codigo.gs`** de esta carpeta. Guardá.

## Paso 4 · Guardar la credencial (no va en el código)

**Configuración del proyecto** (el engranaje de la izquierda) **› Propiedades del script ›
Agregar propiedad**:

| Propiedad | Valor |
|---|---|
| `GPVDP_URL` | `https://finanzas.corporativovdp.com` |
| `GPVDP_TOKEN` | la credencial del paso 1 |
| `GPVDP_DESDE` | *(opcional)* `2026/09/01` para acotar el arranque |

La credencial va acá y **no dentro del código** porque el código se copia, se comparte y se
versiona: un token pegado adentro se filtra solo. El script se niega a arrancar si falta alguna de
las dos primeras, y también si la URL no es `https://` —la credencial viaja en la cabecera y sin TLS
iría en claro por la red—.

## Paso 5 · Verificar y autorizar

Volvé a la hoja, recargá la página, y en el menú nuevo:

**📤 Recepción GPVDP › ✔ Verificar la configuración**

La primera vez Google va a pedir permisos: leer y modificar Gmail (para leer los adjuntos y poner
las etiquetas), hacer llamadas a un servicio externo (el ERP) y usar la hoja (la bitácora).
Aceptalos.

Tiene que decir **«Conexión correcta»** y mostrar los dos buzones. Si dice que **no coinciden**, esa
credencial es de otra empresa: pará acá y generá la correcta, porque si seguís el ERP va a rechazar
todo.

> Esta verificación no mueve ningún correo: solo manda el latido, que es una escritura inofensiva.

## Paso 6 · Apagar el script viejo

En el mismo editor de Apps Script, **⏰ Disparadores** (el reloj de la izquierda): borrá el disparador
del extractor anterior (`procesarFacturasXML`).

Si el extractor está en **otro** proyecto de Apps Script de esa misma cuenta, hay que abrirlo y
borrar el disparador allá.

## Paso 7 · La primera corrida, a mano

**📤 Recepción GPVDP › ▶ Procesar correos pendientes**

Después andá al ERP, a **CxP › Recepción**, y mirá:

- **«Última recepción»** tiene que decir «hace un momento»;
- **«Registradas»** son las que se volvieron cuenta por pagar;
- **«Por revisar»** es la cola de errores, con el motivo escrito en palabras.

En la hoja aparece además una pestaña **«Bitácora de envíos»** con lo que se mandó y qué contestó el
ERP.

## Paso 8 · Dejarlo automático

**📤 Recepción GPVDP › ⏱ Instalar el disparador cada hora**

Y listo. De ahí en adelante se puede seguir desde **CxP › Buzones**, columna **«Última señal»**: si
pasa más de un día sin dar señales, se pinta en rojo.

---

## La prueba que conviene hacer una vez

Es la que verifica lo único que no se puede ver a simple vista: **que un fallo no pierda facturas.**

1. Apagá el backend del ERP: `docker compose stop backend`
2. Corré **▶ Procesar correos pendientes** con al menos un correo con factura sin procesar.
3. En Gmail, revisá que **ningún** hilo quedó con `Procesado-XML`. Deberían tener `Error-Envio`.
4. Levantá el backend: `docker compose start backend`
5. Corré de nuevo. Las facturas entran, y `Error-Envio` desaparece de esos hilos.

Si en el paso 3 aparece algún hilo etiquetado `Procesado-XML`, **pará**: es el bug del extractor
viejo y esa factura se perdería. Avisame.

---

## Qué hacer cuando algo no entra

Nada se pierde: lo que no se pudo registrar queda en **CxP › Recepción**, filtro **«Por revisar»**,
con el motivo. Los más comunes:

| Lo que dice el ERP | Qué pasó | Qué hacer |
|---|---|---|
| «el receptor del comprobante (…) no corresponde a esta empresa» | El proveedor facturó a otra sociedad del grupo, o se equivocó de correo | Reenviar el correo al buzón correcto. Esa recepción **no conserva el XML** a propósito, para no guardar documentos de otra empresa |
| «el token no corresponde a este buzón» | La credencial es de otra empresa | Generar la credencial correcta (paso 1) y volver a pegarla |
| «esta empresa no tiene cédulas jurídicas configuradas» | No debería pasar: las tres se cargaron con la migración 0081 | Avisar |
| «es un comprobante de tipo «Nota de crédito»…» | Correcto y esperado | Nada. Se conserva para cuando se apruebe el manejo de notas de crédito |
| «el XML no se pudo leer» | El adjunto llegó dañado | Pedirle al proveedor que lo reenvíe |
| «esta factura ya está registrada en otra empresa del grupo» | La misma factura entró por dos buzones | Definir a cuál empresa corresponde antes de registrarla |
| «factura en USD sin tipo de cambio» | El comprobante no declara el TC | Pedir el comprobante corregido |

Cuando el motivo se resuelve, el botón **Reintentar** reprocesa con el XML que ya está guardado: no
hace falta volver a pedirle nada al correo.

## Si la credencial se pierde o se filtra

**CxP › Buzones › Rotar credencial.** La anterior deja de servir **de inmediato** —está probado— y
el conector queda sin entregar hasta que pegues la nueva en el paso 4. Los correos de ese rato quedan
en el buzón y entran cuando se reconecte: no se pierde nada.

## Detalles que conviene tener presentes

- **El conector procesa hasta 20 hilos por corrida.** Apps Script corta a los 6 minutos, y con 20
  hay margen de sobra. Si hubiera acumulación, corre cada hora hasta ponerse al día.
- **El PDF es opcional.** Se empareja con su XML por nombre; si en un mismo correo vienen varios de
  cada uno y no hay forma de emparejarlos con certeza, la factura entra **solo con el XML**. Es
  deliberado: es mejor sin PDF que con el PDF de otra factura, que llevaría a pagar mirando el
  documento equivocado.
- **Reintentar es seguro.** El ERP deduplica por Message-ID + la clave del comprobante y contesta
  «repetido» sin crear nada.
- **La bitácora de la hoja es la otra mitad de la conciliación.** El ERP guarda el Message-ID de cada
  recepción, así que comparar «qué mandé» contra «qué tiene el ERP» da exactamente la lista de lo que
  faltara.

## Pendiente conocido

**`/v1/healthz` del ERP devuelve `ok` sin tocar la base de datos.** Si algún día se quiere que el
conector prechequee antes de enviar, ese endpoint hay que arreglarlo primero: hoy daría verde
mientras todos los envíos mueren con 500.
