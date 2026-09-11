/**
 * CxP — Buzones de recepción (/cxp/fuentes).
 *
 * Da de alta el correo de cada empresa y genera su credencial.
 *
 * Dos cosas mandan el diseño de esta pantalla:
 *
 *  1. EL TOKEN SE VE UNA SOLA VEZ. La base guarda solo su hash, así que no hay forma de volver a
 *     mostrarlo: si se pierde, se rota. Por eso el token aparece en un bloque que no se puede
 *     ignorar, con botón de copiar, y no en una celda de la tabla.
 *  2. EL LATIDO ES EL DATO IMPORTANTE. «Última señal: hace 3 días» es lo único que distingue «no
 *     hubo facturas» de «el script está muerto». Va en su propia columna y se pinta cuando lleva
 *     demasiado tiempo callado.
 */

import { useState } from "react";
import { AlertTriangle, Check, Copy, KeyRound, Power } from "lucide-react";
import {
  Badge,
  Button,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  ConfirmDialog,
  EmptyState,
  ErrorState,
  Input,
  LoadingState,
  PageHeader,
  TBody,
  TD,
  TH,
  THead,
  Table,
  TableContainer,
  TR,
  useToast,
} from "@/components/ui";
import { cn } from "@/lib/cn";
import { aFechaHora } from "@/lib/format";
import { mensajeError } from "@/lib/apiError";
import {
  useCambiarEstadoFuente,
  useCrearFuente,
  useFuentes,
  useRotarTokenFuente,
} from "@/features/cxp/hooks";
import type { FuenteRecepcion } from "@/api/cxp";

/** A partir de acá el silencio es sospechoso: el script corre al menos una vez al día. */
const HORAS_SIN_SENAL_SOSPECHOSO = 36;

export function FuentesRecepcionPage() {
  const toast = useToast();
  const query = useFuentes();
  const crear = useCrearFuente();
  const rotar = useRotarTokenFuente();
  const estado = useCambiarEstadoFuente();

  const [nombre, setNombre] = useState("");
  const [correo, setCorreo] = useState("");
  /** El token recién generado, con el contexto de a qué buzón pertenece. */
  const [tokenNuevo, setTokenNuevo] = useState<{ token: string; correo: string } | null>(null);
  const [confirmarRotar, setConfirmarRotar] = useState<FuenteRecepcion | null>(null);

  const datos = query.data;
  const sinCedulas = (datos?.cedulas?.length ?? 0) === 0;

  function crearFuente(e: React.FormEvent) {
    e.preventDefault();
    crear.mutate(
      { nombre: nombre.trim(), correo: correo.trim() },
      {
        onSuccess: (res) => {
          setTokenNuevo({ token: res.token, correo: res.fuente.correo });
          setNombre("");
          setCorreo("");
          toast.success(`Buzón ${res.fuente.correo} dado de alta.`);
        },
        onError: (err) => toast.error(mensajeError(err)),
      },
    );
  }

  return (
    <div className="flex flex-col gap-5">
      <PageHeader
        title="Buzones de recepción"
        description="El correo desde el que cada empresa recibe sus facturas electrónicas, y la credencial que usa el conector."
      />

      {/* Sin cédulas la recepción rechaza todo, así que dar de alta un buzón no serviría de nada. */}
      {datos && sinCedulas && (
        <div className="flex items-start gap-3 rounded-lg border border-negativo/40 bg-negativo/5 px-4 py-3">
          <AlertTriangle className="mt-0.5 h-5 w-5 shrink-0 text-negativo" aria-hidden />
          <div className="text-sm text-content">
            <p className="font-medium">Esta empresa no tiene cédulas jurídicas configuradas.</p>
            <p className="mt-0.5 text-content-muted">
              La recepción compara el receptor de cada factura contra la cédula de la empresa, así que
              sin ellas va a rechazar todo lo que llegue. Configuralas antes de conectar el buzón.
            </p>
          </div>
        </div>
      )}

      {/* EL TOKEN, UNA SOLA VEZ. No se puede volver a mostrar: la base guarda solo su hash. */}
      {tokenNuevo && (
        <TokenNuevo
          token={tokenNuevo.token}
          correo={tokenNuevo.correo}
          onCerrar={() => setTokenNuevo(null)}
        />
      )}

      <Card>
        <CardHeader>
          <CardTitle>Conectar un buzón</CardTitle>
          <p className="mt-0.5 text-xs text-content-muted">
            Un buzón por empresa. La credencial que se genera queda atada a{" "}
            <b className="text-content">esta</b> empresa: pegarla en el conector de otra hace que sus
            facturas se rechacen, y el sistema lo dice en la bandeja de recepción.
          </p>
        </CardHeader>
        <CardContent>
          <form className="flex flex-wrap items-end gap-3" onSubmit={crearFuente}>
            <label className="flex flex-col gap-1 text-xs text-content-muted">
              Nombre
              <Input
                value={nombre}
                onChange={(e) => setNombre(e.target.value)}
                placeholder="Buzón de facturación"
                className="w-56"
                required
              />
            </label>
            <label className="flex flex-col gap-1 text-xs text-content-muted">
              Correo del buzón
              <Input
                type="email"
                value={correo}
                onChange={(e) => setCorreo(e.target.value)}
                placeholder="facturas@empresa.com"
                className="w-72"
                required
              />
            </label>
            <Button type="submit" disabled={crear.isPending}>
              {crear.isPending ? "Generando…" : "Dar de alta y generar credencial"}
            </Button>
          </form>
        </CardContent>
      </Card>

      {query.isPending && <LoadingState />}
      {query.isError && (
        <ErrorState message={mensajeError(query.error)} onRetry={() => void query.refetch()} />
      )}

      {datos && (
        <Card>
          <CardHeader>
            <CardTitle>Buzones de esta empresa</CardTitle>
            {datos.cedulas.length > 0 && (
              <p className="mt-0.5 text-xs text-content-muted">
                Se aceptan facturas dirigidas a{" "}
                <b className="font-mono text-content">{datos.cedulas.join(" · ")}</b>
              </p>
            )}
          </CardHeader>
          <CardContent>
            {datos.fuentes.length === 0 ? (
              <EmptyState message="Todavía no hay ningún buzón conectado. Dando de alta uno, las facturas entran solas sin bajar ni subir archivos." />
            ) : (
              <TableContainer>
                <Table>
                  <THead>
                    <TR>
                      <TH>Buzón</TH>
                      <TH>Última señal</TH>
                      <TH className="text-right">Recibidas</TH>
                      <TH className="text-right">Por revisar</TH>
                      <TH>Estado</TH>
                      <TH />
                    </TR>
                  </THead>
                  <TBody>
                    {datos.fuentes.map((f) => (
                      <TR key={f.id} className={cn(!f.activo && "opacity-60")}>
                        <TD>
                          <span className="block font-medium">{f.correo}</span>
                          <span className="block text-xs text-content-muted">{f.nombre}</span>
                        </TD>
                        <TD>
                          <Senal iso={f.ultimo_contacto} />
                        </TD>
                        <TD className="text-right tabular-nums">{f.recibidas}</TD>
                        <TD
                          className={cn(
                            "text-right tabular-nums",
                            f.parqueadas > 0 && "font-semibold text-negativo",
                          )}
                        >
                          {f.parqueadas || "—"}
                        </TD>
                        <TD>
                          <Badge tone={f.activo ? "positivo" : "neutral"}>
                            {f.activo ? "Activo" : "Desactivado"}
                          </Badge>
                        </TD>
                        <TD>
                          <div className="flex justify-end gap-2">
                            <Button
                              variant="secondary"
                              size="sm"
                              onClick={() => setConfirmarRotar(f)}
                              disabled={rotar.isPending}
                            >
                              <KeyRound className="mr-1 h-3.5 w-3.5" aria-hidden />
                              Rotar credencial
                            </Button>
                            <Button
                              variant="secondary"
                              size="sm"
                              onClick={() =>
                                estado.mutate(
                                  { id: f.id, activo: !f.activo },
                                  {
                                    onSuccess: () =>
                                      toast.success(
                                        f.activo
                                          ? `${f.correo} desactivado: su credencial deja de servir.`
                                          : `${f.correo} activado.`,
                                      ),
                                    onError: (err) => toast.error(mensajeError(err)),
                                  },
                                )
                              }
                              disabled={estado.isPending}
                            >
                              <Power className="mr-1 h-3.5 w-3.5" aria-hidden />
                              {f.activo ? "Desactivar" : "Activar"}
                            </Button>
                          </div>
                        </TD>
                      </TR>
                    ))}
                  </TBody>
                </Table>
              </TableContainer>
            )}
          </CardContent>
        </Card>
      )}

      {/* Rotar deja el conector sin servicio hasta que se pegue el token nuevo: se confirma. */}
      {confirmarRotar !== null && (
        <ConfirmDialog
          titulo="Rotar la credencial"
          descripcion={`La credencial actual de ${confirmarRotar.correo} deja de servir de inmediato.`}
          impacto={[
            "El conector deja de entregar facturas hasta que se pegue la nueva",
            "La credencial nueva se muestra UNA sola vez",
            "Los correos que no se puedan entregar quedan en el buzón y entran cuando se reconecte",
          ]}
          textoConfirmar="Rotar"
          tono="peligro"
          pendiente={rotar.isPending}
          onCancelar={() => setConfirmarRotar(null)}
          onConfirmar={() => {
            const f = confirmarRotar;
            setConfirmarRotar(null);
            rotar.mutate(f.id, {
              onSuccess: (res) => {
                setTokenNuevo({ token: res.token, correo: f.correo });
                toast.success("Credencial rotada.");
              },
              onError: (err) => toast.error(mensajeError(err)),
            });
          }}
        />
      )}
    </div>
  );
}

/**
 * El bloque del token. No es un toast ni una celda: hay que poder copiarlo con calma, y si se
 * pierde no se recupera —solo se rota—, así que la pantalla lo dice.
 */
function TokenNuevo({
  token,
  correo,
  onCerrar,
}: {
  token: string;
  correo: string;
  onCerrar: () => void;
}) {
  const toast = useToast();
  const [copiado, setCopiado] = useState(false);
  return (
    <div className="rounded-lg border-2 border-accent bg-accent/5 px-4 py-3">
      <div className="flex items-start justify-between gap-3">
        <div>
          <p className="text-sm font-medium text-content">
            Credencial de <b>{correo}</b>
          </p>
          <p className="mt-0.5 text-xs text-content-muted">
            Copiala ahora: <b className="text-content">no se puede volver a ver</b>. El sistema guarda
            solo su huella, así que si se pierde hay que rotarla y pegar una nueva en el conector.
          </p>
        </div>
        <Button variant="secondary" size="sm" onClick={onCerrar}>
          Ya la guardé
        </Button>
      </div>
      <div className="mt-2 flex items-center gap-2">
        <code className="flex-1 overflow-x-auto rounded border border-border bg-surface px-3 py-2 font-mono text-sm text-content">
          {token}
        </code>
        <Button
          variant="secondary"
          size="sm"
          onClick={async () => {
            try {
              await navigator.clipboard.writeText(token);
              setCopiado(true);
              toast.success("Credencial copiada.");
            } catch {
              // Sin permiso de portapapeles no se puede copiar: se dice, en vez de fingir que sí.
              toast.error("No se pudo copiar. Seleccionala y copiala a mano.");
            }
          }}
        >
          {copiado ? (
            <Check className="mr-1 h-3.5 w-3.5" aria-hidden />
          ) : (
            <Copy className="mr-1 h-3.5 w-3.5" aria-hidden />
          )}
          {copiado ? "Copiada" : "Copiar"}
        </Button>
      </div>
    </div>
  );
}

/**
 * El latido. Un buzón que nunca llamó y uno que dejó de llamar son dos problemas distintos, y los
 * dos se ven igual si solo se muestra una fecha.
 */
function Senal({ iso }: { iso: string | undefined }) {
  if (!iso) {
    return (
      <span className="text-xs text-content-muted">
        nunca
        <span className="block text-content-muted">el conector no se ha conectado</span>
      </span>
    );
  }
  // `aFechaHora`: con `new Date` el desfase «+00» del backend da NaN, y un buzón que sí dio
  // señales se vería como recién contactado (0 horas), que es el error MÁS peligroso acá.
  const d = aFechaHora(iso);
  const horas = d ? (Date.now() - d.getTime()) / 3600000 : Number.POSITIVE_INFINITY;
  const callado = horas > HORAS_SIN_SENAL_SOSPECHOSO;
  return (
    <span className={cn("text-xs", callado ? "font-semibold text-negativo" : "text-content")}>
      {textoHace(horas)}
      {callado && (
        <span className="block font-normal text-negativo">
          lleva más de un día sin dar señales: puede estar apagado
        </span>
      )}
    </span>
  );
}

function textoHace(horas: number): string {
  if (horas < 1) return "hace menos de 1 h";
  if (horas < 24) return `hace ${Math.floor(horas)} h`;
  const d = Math.floor(horas / 24);
  return d === 1 ? "hace 1 día" : `hace ${d} días`;
}
