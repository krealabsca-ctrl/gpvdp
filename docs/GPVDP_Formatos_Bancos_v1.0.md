# Formatos de estado de cuenta por banco — GPVDP [T1]

> Derivado del análisis de 13 muestras reales (junio 2026). Es el contrato que implementa cada
> adaptador (`bank-import-adapters`). **Todas las cuentas pertenecen a la empresa `Valle de Paz`**,
> aunque el titular del archivo diga Jardines, Colinas, Religiosa, Privado de Cartago o COPENAE.
>
> ⚠️ Contiene números de cuenta/IBAN reales de la empresa (necesarios para seed y memoria IBAN).
> No incluir movimientos reales de terceros en el repo.

## Reglas comunes (todas las adaptadores)
- **Detección**: por *firma de contenido* (fila de encabezado + columnas), NO por nombre de hoja (son dinámicos: fechas/GUID).
- **Fila de inicio**: localizar la fila de encabezado y arrancar en la siguiente; descartar "Saldo Inicial".
- **Fin de movimientos**: cortar en la primera fila no-dato: vacía, `TOTAL`, `Total de…`, `Cuadro de Resumen`, `Saldo Final`, o notas al pie.
- **Débito/Crédito**: todos los bancos traen columnas separadas (nunca ambas > 0). Limpiar montos: quitar prefijo de moneda (`CRC `/`USD `), separador de miles `,`, y tratar `-`/vacío como `0`. Dinero → `decimal`, nunca float.
- **Moneda y cuenta**: la cuenta destino del import se elige en la subida (`cuenta_bancaria_id`); la moneda sale de esa cuenta. La moneda/IBAN del archivo (cuando existe) es verificación cruzada.
- **Memoria IBAN** (RN-02): si el archivo trae IBAN, asociar `IBAN → banco → cuenta`. Promerica trae nº de cuenta; BN no trae nada.
- **natural_key** (RN-08): `hash(cuenta_bancaria_id, fecha, débito, crédito, documento, indice_ocurrencia)`. La descripción NO entra.
- **source_file_hash** del archivo sin modificar (RN-06).

---

## 1. Promerica (CRC)  ·  `COLINAS PROMERICA COLONES.xlsx`
- **Firma**: texto `Banco Promerica Costa Rica` en cabecera + fila con `[A]Fecha [B]Documento [C]Descripción [D]Débitos [E]Créditos [F]Saldo`.
- **Encabezado** ≈ fila 8; **datos** desde la 9. Fin: fila vacía → `Total de Débitos`/`Total de Créditos`.
- **Mapeo**: A=Fecha, B=Documento, C=Descripción, D=Débito, E=Crédito, F=Saldo(ignorar).
- **Fecha**: `MM-DD-YY` (`06-16-26`). **Monto**: `361,600.00`.
- **Cuenta**: fila `Número de cuenta: 30000002738132-CUENTA CORRIENTE CORPORATIVA COLONES`. **Sin IBAN**.

## 2. Banco Nacional — BN (CRC y USD)  ·  5 archivos (JARDINES, PRIVADO DE CARTAGO, VALLE DE PAZ ×CRC/USD)
- **Firma**: fila 1 encabezado EXACTO `[A]oficina [B]fechaMovimiento [C]numeroDocumento [D]debito [E]credito [F]descripcion`.
- **Encabezado** fila 1; **datos** desde la 2. Fin: fila con `[C]TOTAL`.
- **Mapeo**: A=oficina(ignorar), B=Fecha, C=Documento, D=Débito, E=Crédito, F=Descripción.
- **Fecha**: `MM-DD-YY`. **Monto**: `15,889.69` (USD: `3.36`).
- **Sin IBAN ni nº de cuenta en el archivo** → la cuenta se define por la subida. La descripción suele traer `.../CONTRAPARTE` y textos `TRASLADO 1989 A 132 FONDOS` (referencias internas del grupo → traslados).

## 3. BAC (CRC)  ·  `RELIGIOSA BAC.xlsx`, `VALLE DE PAZ BAC COLONES.xlsx`
- **Firma**: `DETALLE DE MOVIMIENTOS DEL PERÍODO` + fila `[A]Fecha [B]Referencia [D]Código [E]Descripción [H]Débitos [I]Créditos [J]Balance*`.
- **Encabezado** ≈ fila 13; **datos** desde la 15 (saltar fila 14 `Saldo Inicial`). Fin: vacío → `Cuadro de Resumen`.
- **Mapeo**: A=Fecha, B=Referencia→Documento, D=Código(tipo mov: TF/PP/CP/MD…), E=Descripción, H=Débito, I=Crédito, J=Balance(ignorar).
- **Fecha**: `DD/MM/YYYY`. **Monto**: `467.00`.
- **IBAN**: cabecera `Producto: CR…` (ej. `CR26010200009038253541`, `CR10010200009510302389`). **Moneda**: celda `CRC`.
- **Especial (RN-07)**: BAC puede traer **líneas legítimamente duplicadas** dentro del archivo → conservar ambas con `indice_ocurrencia` y alertar.

## 4. BCR (CRC)  ·  `RELIGIOSA BCR.xlsx`
- **Firma**: `Movimientos de Cuenta` + fila `[A]Fecha Contable [B]Fecha de Registro [C]Hora [D]Número Documento [E]Descripción [F]Oficina [G]Débitos [H]Créditos`.
- **Encabezado** ≈ fila 9; **datos** desde la 10. Fin: vacío → `Total Débitos`/`Saldo Inicial/Final`.
- **Mapeo**: A=Fecha(contable), D=Documento, E=Descripción, G=Débito, H=Crédito. (B/C/F ignorar.)
- **Fecha**: `MM-DD-YY`. **Monto**: `8,700.00`; **débito vacío = `-`** (tratar como 0).
- **IBAN**: fila `Cuenta IBAN: CC-CR48015201349000020206` → quitar prefijo `CC-`. Descripción usa `//` como separador. **Moneda**: no rotulada en el archivo (confirmar; se asume por la cuenta).

## 5. Banco Popular — BP (CRC)  ·  `VALLE DE PAZ BP COLONES.xlsx`
- **Firma**: `Banco Popular` + fila `[A]Fecha y Hora [B]Descripcion [C]Documento [D]Debito [E]Creditos [F]Saldo`.
- **Encabezado** ≈ fila 12; **datos** desde la 15 (saltar 13 vacía y 14 `Saldo Inicial`). Fin: `Saldo Final`.
- **Mapeo**: A=Fecha, B=Descripción, C=Documento, D=Débito, E=Crédito, F=Saldo(ignorar).
- **Fecha**: `DD MON YYYY HH:MM` en español (`01 JUN 2026 03:23`). **Monto con prefijo**: `CRC 6,500.00` (quitar `CRC `).
- **IBAN**: `Cuenta IBAN: CR62016101008810244232` (nº cuenta `0000396169`). **Moneda**: `Colon Costa Rica`.

## 6. Davivienda (CRC y USD)  ·  3 cuentas separadas
Archivos: `…DAVIVIENDA COLONES` (CRC), `…DAVIVIENDA COMISIONES COPENAE` (CRC), `…DAVIVIENDA DOLARES` (USD).
- **Firma**: hoja `Movimmientos` (sic) + fila `[A]Fecha [B]Descripción [C]Ref. [D]Débitos (DR) [E]Créditos (CR) [F]Saldo Contable [G]Ref2 [H]Tipo Tran [I]Causa [J]Sucursal [K]D/C [L]Cuenta`.
- **Encabezado** ≈ fila 13; **datos** desde la 14.
- **Mapeo**: A=Fecha, B=Descripción, C=Ref→Documento, D=Débito, E=Crédito, F=Saldo(ignorar). Metadatos útiles: H=Tipo Tran, I=Causa, K=D/C, L=IBAN propio.
- **Fecha**: `DD/MM/YYYY`. **Monto**: `7,754,527.33`.
- **IBAN**: cabecera `Número de Cuenta: CR…` (Colones `CR76010409142215626710`, COPENAE `CR17010402842201520116`, Dólares `CR74010409142215627425`). **Moneda**: cabecera `Colones`/`Dolares`.
- **Especial**:
  - **Overnight/Traslado**: descripción `TRF DE/A CR…, (DESINV./INV. OVERNIGHT)` y el IBAN de la contraparte embebido → alimenta el emparejamiento (RN-19).
  - **Consecutivo Largo (export §30)**: extraer 25 dígitos desde la posición 24 de la descripción (`=EXTRAE(desc;24;25)`). Solo aplica a exportación.

---

## Inventario de cuentas a sembrar (empresa: Valle de Paz)

| Banco | Alias / cuenta | IBAN / Nº | Moneda | Archivo muestra |
|---|---|---|---|---|
| Promerica | Colinas (corriente corp.) | nº `30000002738132` | CRC | COLINAS PROMERICA COLONES |
| BN | Jardines Colones | (sin IBAN) | CRC | JARDINES BN COLONES |
| BN | Jardines Dólares | (sin IBAN) | USD | JARDINES BN DOLARES |
| BN | Privado de Cartago Colones | (sin IBAN) | CRC | PRIVADO DE CARTAGO BN COLONES |
| BN | Valle de Paz Colones | (sin IBAN) | CRC | VALLE DE PAZ BN COLONES |
| BN | Valle de Paz Dólares | (sin IBAN) | USD | VALLE DE PAZ BN DOLARES |
| BAC | Religiosa | `CR26010200009038253541` | CRC | RELIGIOSA BAC |
| BAC | Valle de Paz Colones | `CR10010200009510302389` | CRC | VALLE DE PAZ BAC COLONES |
| BCR | Religiosa | `CR48015201349000020206` | CRC (confirmar) | RELIGIOSA BCR |
| BP | Valle de Paz Colones | `CR62016101008810244232` | CRC | VALLE DE PAZ BP COLONES |
| Davivienda | Colones | `CR76010409142215626710` | CRC | VALLE DE PAZ DAVIVIENDA COLONES |
| Davivienda | Comisiones COPENAE | `CR17010402842201520116` | CRC | VALLE DE PAZ DAVIVIENDA COMISIONES COPENAE |
| Davivienda | Dólares | `CR74010409142215627425` | USD | VALLE DE PAZ DAVIVIENDA DOLARES |

> Davivienda = **1 banco** con **3 cuentas** distinguidas por alias (el usuario las "registra por separado según el nombre").
> BN = 5 cuentas sin IBAN → se identifican por la cuenta elegida al importar (memoria IBAN no aplica a BN).

## Pendientes / a confirmar
- Moneda de `RELIGIOSA BCR` (no rotulada en el archivo).
- **TOLERANCIA_TRASLADO** (semántica del "98%") y **CIERRE_PERIODO_BLOQUEANTE** — ver `CLAUDE.md`.
- Reglas de clasificación iniciales por palabra clave (se aprenden en "Revisar"; se pueden sembrar algunas).
