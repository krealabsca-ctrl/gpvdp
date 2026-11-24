/**
 * «Llenarlo en Excel»: sacar el mapeo, llenarlo afuera y volverlo a subir (12-nov-2026).
 *
 * POR QUÉ EXISTE. Quien sabe a qué cuenta va cada partida es el contador, y el mapeo solo se podía
 * decidir adentro del sistema, pantalla por pantalla. Es el mismo problema que ya resolvió el
 * diccionario del catálogo de Bancos y la misma solución: el trabajo se arma donde la gente
 * trabaja y se sube de una vez.
 *
 * EL ARCHIVO NO SUGIERE NADA, y es una decisión medida. Proponer la cuenta comparando nombres
 * —como se hace con las cuentas bancarias por IBAN— acierta 6 de 131 contra el plan real, y en los
 * parciales «Alquiler» devuelve NUEVE candidatos que mezclan cuentas de gasto, de ingreso y de
 * activo. Una sugerencia así se acepta en lote y produce un error contable que nadie revisa.
 *
 * LOS RECHAZOS VAN CON EL NÚMERO DE FILA DEL EXCEL. Un «no se pudieron aplicar 12» sobre una hoja
 * de 300 filas obliga a revisarla entera a ojo; el número de fila es lo único accionable.
 */
import { useRef, useState } from "react";

import {
  Badge,
  Button,
  Card,
  CardContent,
  CardDescription,
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
import { contaApi, type ResultadoImportacionMapeo } from "@/api/conta";
import { useImportarMapeoXLSX } from "@/features/conta/hooks";
import { mensajeError } from "@/lib/apiError";
import { descargarBlob } from "@/lib/csv";

export function MapeoExcelPanel() {
  const toast = useToast();
  const inputRef = useRef<HTMLInputElement>(null);
  const [bajando, setBajando] = useState(false);
  const [res, setRes] = useState<ResultadoImportacionMapeo | null>(null);

  async function bajar() {
    setBajando(true);
    try {
      descargarBlob("mapeo-contable.xlsx", await contaApi.exportarMapeoXLSX());
    } catch (e) {
      toast.error(mensajeError(e));
    } finally {
      setBajando(false);
    }
  }

  const subir = useImportarMapeoXLSX();

  function alSubir(archivo: File) {
    setRes(null);
    subir.mutate(archivo, {
      onSuccess: (r) => {
        setRes(r);
        if (r.aplicadas > 0) {
          toast.success(`${r.aplicadas} partida(s) decidida(s) desde el archivo`);
        } else if (r.rechazos.length > 0) {
          toast.error("No entró ninguna fila: revisá los rechazos");
        }
      },
      onError: (e) => toast.error(mensajeError(e)),
    });
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Llenarlo en Excel</CardTitle>
        <CardDescription>
          Bajá la hoja con las partidas que faltan —ordenadas por la plata que mueven— y el plan de
          cuentas en la segunda hoja. Se llena la columna <strong>Código de cuenta</strong>, o el{" "}
          <strong>motivo</strong> si esa partida no se contabiliza, y se vuelve a subir. Lo que ya
          está decidido viaja también, así que el archivo sirve para revisar y corregir.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="flex flex-wrap items-center gap-3">
          <Button variant="secondary" onClick={() => void bajar()} loading={bajando}>
            Bajar la hoja (.xlsx)
          </Button>
          <input
            ref={inputRef}
            type="file"
            accept=".xlsx"
            className="hidden"
            aria-label="Archivo del mapeo"
            onChange={(e) => {
              const archivo = e.target.files?.[0];
              if (archivo) {
                alSubir(archivo);
              }
              // Se limpia para que subir el MISMO archivo dos veces vuelva a disparar el evento:
              // corregir la hoja y reintentar es el caso normal, no la excepción.
              e.target.value = "";
            }}
          />
          <Button onClick={() => inputRef.current?.click()} loading={subir.isPending}>
            Subir la hoja llena
          </Button>
        </div>
        <p className="text-xs text-content-muted">
          La columna de la cuenta sale <strong>vacía</strong>: el sistema no adivina a qué cuenta va
          cada partida. Esa decisión es contable y queda escrita con tu nombre.
        </p>

        {res && (
          <div className="space-y-3 rounded-lg border border-border p-4">
            <div className="flex flex-wrap items-center gap-3 text-sm">
              <Badge tone={res.aplicadas > 0 ? "positivo" : "neutral"}>
                {res.aplicadas} aplicada(s)
              </Badge>
              {res.resumen.mapeadas > 0 && (
                <span className="text-content-muted">{res.resumen.mapeadas} con cuenta</span>
              )}
              {res.resumen.no_contables > 0 && (
                <span className="text-content-muted">
                  {res.resumen.no_contables} marcadas «no se contabiliza»
                </span>
              )}
              <span className="text-content-muted">
                de {res.resumen.filas_leidas} fila(s) con decisión en el archivo
              </span>
            </div>

            {res.rechazos.length > 0 && (
              <>
                <p className="text-sm">
                  <strong>{res.rechazos.length} fila(s) no entraron.</strong> El resto sí: corregí
                  estas en el archivo y volvé a subirlo, no hace falta empezar de nuevo.
                </p>
                <TableContainer>
                  <Table>
                    <THead>
                      <TR>
                        <TH className="w-20 text-right">Fila</TH>
                        <TH>Partida</TH>
                        <TH>Qué pasó</TH>
                      </TR>
                    </THead>
                    <TBody>
                      {res.rechazos.map((r) => (
                        <TR key={`${r.fila}-${r.clasificacion}`}>
                          <TD className="text-right tabular-nums">{r.fila}</TD>
                          <TD>
                            <span className="text-content-muted">{r.concepto} › </span>
                            {r.clasificacion}
                          </TD>
                          <TD className="text-sm">{r.motivo}</TD>
                        </TR>
                      ))}
                    </TBody>
                  </Table>
                </TableContainer>
              </>
            )}
          </div>
        )}
      </CardContent>
    </Card>
  );
}
