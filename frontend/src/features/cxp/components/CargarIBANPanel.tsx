/**
 * Carga masiva de cuentas IBAN de proveedores.
 *
 * Por qué existe: la macro de pago lleva el IBAN en el primer campo y el banco rechaza la línea
 * sin él. Al medirlo había 648 proveedores sin cuenta, así que la macro salía inservible.
 * Completarlos de a uno desde la ficha no es viable.
 *
 * Se PEGA desde Excel en vez de subir un archivo: es lo que la gente hace naturalmente
 * (seleccionar dos columnas y Ctrl+C), no necesita que el archivo tenga un formato exacto, y se
 * ve al instante qué se entendió de cada fila. Primero previsualiza, después confirma — nunca
 * escribe sin mostrar antes lo que va a pasar.
 */

import { useMemo, useState } from "react";
import {
  Badge,
  Button,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  Table,
  TableContainer,
  TBody,
  TD,
  TH,
  THead,
  TR,
  useToast,
} from "@/components/ui";
import { mensajeError } from "@/lib/apiError";
import { formatMonto } from "@/lib/format";
import { useCargarIBAN, usePrevisualizarIBAN, useProveedoresSinIBAN } from "@/features/cxp/hooks";
import type { FilaIBAN } from "@/api/cxp";

/** Separa lo pegado en filas de (identificación, IBAN). Acepta tabulaciones, comas o punto y coma. */
function parsearPegado(texto: string): FilaIBAN[] {
  const filas: FilaIBAN[] = [];
  const lineas = texto.split(/\r?\n/);
  for (let i = 0; i < lineas.length; i++) {
    const linea = lineas[i]?.trim();
    if (!linea) continue;
    // Excel pega con tabulaciones; un .csv guardado a mano usa coma o punto y coma.
    const partes = linea.split(/\t|;|,/).map((p) => p.trim());
    // Si la primera línea es el encabezado del Excel, se ignora.
    if (i === 0 && /identific|cedula|c[eé]dula/i.test(partes[0] ?? "") ) continue;
    // La columna del IBAN es la que empieza con CR; si no hay ninguna, se toma la segunda.
    const iban = partes.find((p) => /^CR/i.test(p.replace(/[\s-]/g, ""))) ?? partes[1] ?? "";
    const identificacion = partes[0] ?? "";
    if (!identificacion && !iban) continue;
    filas.push({ fila: i + 1, identificacion, iban, nombre: "", estado: "", detalle: "", proveedor_id: "", iban_anterior: "" });
  }
  return filas;
}

const TONO: Record<string, "positivo" | "negativo" | "pendiente" | "accent"> = {
  OK: "positivo",
  SIN_CAMBIO: "accent",
  INVALIDO: "negativo",
  NO_ENCONTRADO: "negativo",
  DUPLICADO: "pendiente",
};

const ETIQUETA: Record<string, string> = {
  OK: "se carga",
  SIN_CAMBIO: "ya lo tenía",
  INVALIDO: "revisar",
  NO_ENCONTRADO: "no existe",
  DUPLICADO: "repetida",
};

export function CargarIBANPanel() {
  const toast = useToast();
  const [texto, setTexto] = useState("");
  const preview = usePrevisualizarIBAN();
  const cargar = useCargarIBAN();
  const sinIBAN = useProveedoresSinIBAN();

  const filas = useMemo(() => parsearPegado(texto), [texto]);
  const resumen = preview.data;

  function previsualizar() {
    if (!filas.length) {
      toast.error("Pegá al menos una fila con la identificación y el IBAN.");
      return;
    }
    preview.mutate(filas, { onError: (e) => toast.error(mensajeError(e)) });
  }

  function confirmar() {
    cargar.mutate(filas, {
      onSuccess: (r) => {
        toast.success(`${r.actualizados} proveedor(es) con cuenta cargada.`);
        setTexto("");
        preview.reset();
      },
      onError: (e) => toast.error(mensajeError(e)),
    });
  }

  const faltan = sinIBAN.data?.total ?? 0;

  return (
    <Card>
      <CardHeader>
        <CardTitle>Cuentas IBAN de proveedores</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-3">
        {faltan > 0 && (
          <div className="flex items-start gap-2 rounded-lg border border-negativo/40 bg-negativo/5 px-3 py-2 text-sm text-content">
            ⚠{" "}
            <span>
              <b>{formatMonto(String(faltan))} proveedor(es) sin cuenta IBAN.</b> El banco rechaza
              la línea de pago sin cuenta destino, así que esas facturas no se pueden pagar por
              transferencia hasta cargarla. El sistema no deja generar la macro con ellas dentro.
            </span>
          </div>
        )}

        <div className="text-sm text-content-muted">
          Copiá de Excel dos columnas —<b className="text-content">identificación</b> y{" "}
          <b className="text-content">IBAN</b>— y pegalas acá. Da igual si el IBAN trae espacios o
          guiones, y si la primera fila es el título de la columna se ignora.
        </div>

        <textarea
          value={texto}
          onChange={(e) => setTexto(e.target.value)}
          rows={6}
          spellCheck={false}
          placeholder={"3101402954\tCR21 0151 0001 0026 2841 12\n402310892\tCR21015100010026284113"}
          className="w-full rounded-lg border border-border bg-surface-raised px-3 py-2 font-mono text-xs text-content placeholder:text-content-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent"
        />

        <div className="flex flex-wrap items-center gap-2">
          <Button onClick={previsualizar} disabled={!filas.length || preview.isPending}>
            {preview.isPending ? "Revisando…" : `Revisar ${filas.length || ""} fila(s)`}
          </Button>
          {resumen && (
            <Button
              variant="secondary"
              onClick={confirmar}
              disabled={resumen.a_cargar === 0 || cargar.isPending}
            >
              {cargar.isPending ? "Cargando…" : `Cargar ${resumen.a_cargar} cuenta(s)`}
            </Button>
          )}
          {texto && (
            <button
              type="button"
              onClick={() => {
                setTexto("");
                preview.reset();
              }}
              className="text-xs text-accent underline"
            >
              Limpiar
            </button>
          )}
        </div>

        {resumen && (
          <>
            <div className="flex flex-wrap gap-3 text-xs text-content-muted">
              <span>
                <b className="text-positivo">{resumen.a_cargar}</b> se cargan
              </span>
              <span>
                <b className="text-content">{resumen.sin_cambio}</b> ya la tenían
              </span>
              <span>
                <b className="text-negativo">{resumen.invalidos}</b> con el IBAN mal escrito
              </span>
              <span>
                <b className="text-negativo">{resumen.no_hallados}</b> sin proveedor que calce
              </span>
              <span>
                <b className="text-pendiente">{resumen.duplicados}</b> repetidas
              </span>
            </div>
            <TableContainer>
              <Table>
                <THead>
                  <TR>
                    <TH>Fila</TH>
                    <TH>Identificación</TH>
                    <TH>Proveedor</TH>
                    <TH>IBAN</TH>
                    <TH>Qué pasa</TH>
                  </TR>
                </THead>
                <TBody>
                  {resumen.filas.map((f) => (
                    <TR key={`${f.fila}-${f.identificacion}`}>
                      <TD className="text-sm text-content-muted">{f.fila}</TD>
                      <TD className="font-mono text-xs">{f.identificacion || "—"}</TD>
                      <TD className="text-sm">{f.nombre || "—"}</TD>
                      <TD className="font-mono text-xs">
                        {f.iban || "—"}
                        {f.iban_anterior && f.iban_anterior !== f.iban && (
                          <span className="mt-0.5 block text-content-muted">
                            antes: {f.iban_anterior}
                          </span>
                        )}
                      </TD>
                      <TD className="text-sm">
                        <Badge tone={TONO[f.estado] ?? "pendiente"}>
                          {ETIQUETA[f.estado] ?? f.estado}
                        </Badge>
                        {f.detalle && (
                          <span className="mt-0.5 block text-xs text-content-muted">{f.detalle}</span>
                        )}
                      </TD>
                    </TR>
                  ))}
                </TBody>
              </Table>
            </TableContainer>
          </>
        )}
      </CardContent>
    </Card>
  );
}
