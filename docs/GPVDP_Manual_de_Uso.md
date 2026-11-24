# GPVDP — Manual de uso

**El manual vive adentro del sistema.** Entrá y hacé clic en el signo de pregunta de la barra
superior, o andá directo a `/ayuda`.

Este archivo ya no tiene el texto del manual, y es a propósito: eran dos textos que mantener, y el
segundo siempre es el que queda viejo. La fuente única es
`frontend/src/features/ayuda/guias.ts` (las guías por tarea) y
`frontend/src/features/ayuda/pantallas.ts` (la ayuda de cada pantalla).

## Qué hay adentro

**Ayuda de cada pantalla.** El botón `?` al lado del título de cualquier pantalla explica qué se
hace ahí y las tres o cuatro cosas que conviene saber. Aparece solo donde hay algo escrito: un `?`
que abre un panel vacío enseña a no hacer clic.

**El manual completo**, en `/ayuda`, ordenado por lo que hay que hacer y no por módulo:

- Cómo está organizado el sistema
- El cierre de mes, en orden
- Pagarle a un proveedor, de principio a fin
- Cuando una factura no sale derecho
- Cuando el sistema no reconoce el archivo de un banco
- Buscar y ordenar en cualquier lista
- Quién puede qué, y qué hacer si no podés entrar
- Las cosas que conviene saber antes de que pasen
- Cuando una pantalla da error

Tiene buscador sobre el texto completo —nadie busca «Pagarle a un proveedor», busca «macro»— y un
botón para imprimirlo, que es lo que sirve para mandárselo a alguien que todavía no tiene usuario.

## Para quien edita el manual

Las guías son datos tipados, no markdown: cada una declara a qué pantallas corresponde, y eso es lo
que hace que la ayuda aparezca sola donde está la duda. Los tipos están en
`frontend/src/features/ayuda/tipos.ts`.

Una prueba (`features/ayuda/ayuda.test.ts`) lee el router y exige que toda ruta nombrada por la
ayuda exista. Si alguien renombra una pantalla, la prueba falla en vez de dejar la ayuda invisible.

## Lo que no está acá

Cómo **instalar** el ERP en un servidor: eso es `deploy/LEEME-VPS.md`, y es otro público.

*Describe el sistema al 22 de octubre de 2026 (migración 108).*
