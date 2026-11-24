/**
 * «A este proveedor le creemos a la ficha, no al comprobante» — en lote.
 *
 * EL PEDIDO (Director Financiero, 21-oct-2026): «cuando había una factura bloqueada porque salía
 * que era de contado pero en realidad era a crédito, hay proveedores que son así y es tedioso cada
 * factura ejecutar el mismo proceso de limpia; lo mejor es que exista un botón del proveedor que a
 * pesar de que dice contado es a crédito».
 *
 * LA REGLA YA EXISTÍA Y NADIE LA TENÍA PUESTA. Está desde la migración 0097 en el INSERT del
 * documento, y al medirlo: 0 de 649 proveedores con el convenio marcado, contra 3.565 de 4.542
 * facturas que declaran «contado». No estaba mal hecha: estaba escondida detrás de un campo que se
 * llamaba «el vencimiento lo manda» y no decía que también gobierna el candado del contado.
 *
 * POR QUÉ UN PANEL Y NO UN BOTÓN POR FICHA. El botón por ficha ya existe (es el selector de la
 * ficha del proveedor). Lo que faltaba es el camino inverso: no «abrí este proveedor y decidí»,
 * sino «mostrame cuáles son». Con 588 proveedores que han declarado contado, abrirlos de a uno
 * para ver a cuáles les toca es el mismo trabajo tedioso que el pedido quiere eliminar, un nivel
 * más arriba.
 *
 * LO QUE NO HACE SOLO: liberar. Marcar el convenio arregla las facturas que entren de ahora en
 * adelante; las que ya están trancadas llevan el candado escrito en cada una. Liberarlas es
 * afirmar «ésta no se pagó por caja chica», que es la única barrera que queda contra el doble
 * pago. Por eso se ofrece aparte, con la cantidad y el monto a la vista, y con motivo obligatorio.
 */

import { useMemo, useState } from "react";

import {
  Badge,
  Button,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  Input,
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
import { formatMoneda, formatMonto } from "@/lib/format";
import { useTienePermiso } from "@/features/auth/permisos";
import {
  useBloqueadasDeProveedores,
  useCandidatosConvenio,
  useLiberarDeProveedores,
  useMarcarConvenioMasivo,
} from "@/features/cxp/hooks";
import type { CandidatoConvenio } from "@/api/cxp";

/** Mínimo del motivo. El mismo del servidor y el de la ficha: descarta el «ok» reflejo. */
const MOTIVO_MINIMO = 20;

/** Lo que queda pendiente de liberar después de marcar: lo que se ofrece confirmar. */
interface PorLiberar {
  ids: string[];
  bloqueadas: number;
  montoCRC: string;
}

export function ConvenioEnLotePanel() {
  const toast = useToast();
  const tiene = useTienePermiso();
  const puedeMarcar = tiene("cxp.parametros");
  const puedeLiberar = tiene("cxp.liberar_pago");

  const [abierto, setAbierto] = useState(false);
  const [verTodos, setVerTodos] = useState(false);
  const [elegidos, setElegidos] = useState<Set<string>>(new Set());
  const [porLiberar, setPorLiberar] = useState<PorLiberar | null>(null);
  const [motivo, setMotivo] = useState("");
  /**
   * Completar la segunda condición en los que están en CONTADO. MARCADA por defecto, igual que al
   * liberar desde la ficha: sin esto el botón dejaba el trabajo a medias en 573 de 649 proveedores
   * y mandaba a corregirlos a mano, que es el tedio que el panel venía a eliminar.
   *
   * Se puede desmarcar porque entre los que declaran contado hay dos clases: el que lo declara por
   * costumbre y es a crédito, y el que de verdad se paga en el mostrador. Para el segundo el
   * candado corresponde.
   */
  const [pasarACredito, setPasarACredito] = useState(true);

  // La lista solo se pide al abrir: es un GROUP BY sobre las 4.542 facturas y no tiene por qué
  // correr en cada visita al maestro de proveedores.
  const candidatosQ = useCandidatosConvenio(!verTodos, abierto);
  const marcar = useMarcarConvenioMasivo();
  const contar = useBloqueadasDeProveedores();
  const liberar = useLiberarDeProveedores();

  const candidatos = useMemo(() => candidatosQ.data ?? [], [candidatosQ.data]);

  // El atajo del pedido: los que la ficha YA tiene en crédito y siguen declarando contado. Para
  // ellos marcar el convenio no cambia ningún plazo —ya es el de la ficha—, solo levanta el
  // candado. Es el grupo donde la decisión no tiene nada que sopesar.
  const yaEnCredito = useMemo(
    () => candidatos.filter((c) => c.condicion_pago === "CREDITO" && c.convenio_manda !== "PROVEEDOR"),
    [candidatos],
  );

  const seleccionados = candidatos.filter((c) => elegidos.has(c.proveedor_id));
  const bloqueadasDeLaSeleccion = seleccionados.reduce((s, c) => s + c.bloqueadas, 0);
  // Cuántos de los elegidos cambiarían de condición. Se dice ANTES de apretar: es lo que
  // convierte la casilla en una decisión informada y no en una letra chica.
  const enContadoDeLaSeleccion = seleccionados.filter((c) => c.condicion_pago !== "CREDITO").length;

  function alternar(id: string) {
    setElegidos((prev) => {
      const s = new Set(prev);
      if (s.has(id)) s.delete(id);
      else s.add(id);
      return s;
    });
  }

  /**
   * Marca el convenio y, si queda algo trancado, ofrece liberarlo.
   *
   * Los dos pasos van encadenados pero separados: el primero ya quedó guardado cuando se pregunta
   * por el segundo, así que cancelar la liberación no deshace el marcado —ni tiene por qué—.
   */
  function marcarElegidos() {
    const ids = seleccionados.map((c) => c.proveedor_id);
    if (ids.length === 0) return;
    marcar.mutate(
      { ids, convenio: "PROVEEDOR", pasarACredito },
      {
        onSuccess: (r) => {
          toast.success(
            `${r.actualizados} proveedor(es) marcados: sus facturas nuevas ya no nacerán trancadas.`,
          );
          // EL CAMBIO DE CONDICIÓN SE ANUNCIA APARTE. No es «a quién se le cree»: es lo que el
          // proveedor ES para el sistema, y de ahí en adelante sus facturas se tratan como de
          // crédito. Mezclarlo en el «N marcados» lo volvería un detalle.
          if (r.pasados_a_credito.length > 0) {
            toast.info(
              `${r.pasados_a_credito.length} pasaron de «Contado» a «Crédito», que es lo que activa la regla. ` +
                `El plazo de crédito no se tocó: si lo tenés pactado, escribilo en su ficha.`,
            );
          }
          // LA TRAMPA, cuando se eligió NO pasarlos: con la ficha en CONTADO la regla NO se
          // activa, y callarlo deja a quien marcó creyendo que lo resolvió hasta que la próxima
          // factura vuelva trancada.
          if (r.sin_credito.length > 0) {
            toast.error(
              `${r.sin_credito.length} quedaron a medias: su ficha dice «Contado», y con eso la regla no se activa. ` +
                `Pasalos a Crédito con su plazo: ${r.sin_credito.slice(0, 3).join(", ")}` +
                (r.sin_credito.length > 3 ? "…" : ""),
            );
          }
          setElegidos(new Set());
          // Lo que YA está trancado no se mueve con esto. Se cuenta ahora para ofrecerlo, porque
          // si no se ofrece acá nadie va después a otra pantalla a buscarlo.
          contar.mutate(ids, {
            onSuccess: (c) => {
              if (c.bloqueadas > 0) setPorLiberar({ ids, bloqueadas: c.bloqueadas, montoCRC: c.monto_crc });
            },
            // Que falle el conteo no invalida el marcado, que ya quedó guardado. Se avisa y punto.
            onError: () =>
              toast.info("Se marcó el convenio. No se pudo contar lo ya trancado: revisalo en cada ficha."),
          });
        },
        onError: (e) => toast.error(mensajeError(e)),
      },
    );
  }

  function liberarLoTrancado() {
    if (!porLiberar) return;
    liberar.mutate(
      { ids: porLiberar.ids, motivo },
      {
        onSuccess: (r) => {
          toast.success(`${r.liberadas} factura(s) liberadas: ya se pueden pagar por banco.`);
          setPorLiberar(null);
          setMotivo("");
        },
        onError: (e) => toast.error(mensajeError(e)),
      },
    );
  }

  // Sin permiso para marcar no se pinta el panel: sería un listado que no lleva a ninguna acción.
  // El selector de la ficha del proveedor sigue ahí para quien sí lo tenga.
  if (!puedeMarcar) return null;

  const total = candidatos.length;

  return (
    <Card>
      <CardHeader>
        <CardTitle>Proveedores que declaran «contado» en sus comprobantes</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-3">
        <p className="text-sm text-content-muted">
          Una factura que el comprobante declara de contado nace <b className="text-content">trancada
          para pago por banco</b>: pudo haber salido ya por caja chica, y el candado es lo que evita
          pagarla dos veces. Cuando el proveedor <i>siempre</i> declara contado pero el convenio es a
          crédito, lo que corresponde no es destrancar factura por factura, sino decir de una vez{" "}
          <b className="text-content">a quién se le cree sobre ese proveedor</b>.
        </p>

        {!abierto ? (
          <div>
            <Button variant="secondary" onClick={() => setAbierto(true)}>
              Ver a cuáles les pasa
            </Button>
          </div>
        ) : candidatosQ.isPending ? (
          <p className="text-sm text-content-muted">Revisando los comprobantes…</p>
        ) : candidatosQ.isError ? (
          <p className="text-sm text-negativo">{mensajeError(candidatosQ.error)}</p>
        ) : total === 0 ? (
          <p className="text-sm text-content-muted">
            {verTodos
              ? "Ningún proveedor ha declarado «contado» en sus comprobantes."
              : "No queda ninguno pendiente: todos los que declaran contado ya tienen el convenio puesto."}
          </p>
        ) : (
          <>
            <div className="flex flex-wrap items-center gap-2">
              <span className="text-sm text-content">
                <b>{formatMonto(String(total))}</b> proveedor(es)
                {verTodos ? "" : " sin resolver"}
              </span>
              {yaEnCredito.length > 0 && (
                <Button
                  size="sm"
                  variant="secondary"
                  onClick={() => setElegidos(new Set(yaEnCredito.map((c) => c.proveedor_id)))}
                  title="Su ficha ya está en crédito: marcarles el convenio no cambia ningún plazo, solo deja de trancarles las facturas"
                >
                  Elegir los {yaEnCredito.length} que ya están en crédito
                </Button>
              )}
              {elegidos.size > 0 && (
                <Button size="sm" variant="ghost" onClick={() => setElegidos(new Set())}>
                  Quitar la selección
                </Button>
              )}
              <label className="ml-auto flex items-center gap-2 text-xs text-content-muted">
                <input
                  type="checkbox"
                  checked={verTodos}
                  onChange={(e) => {
                    setVerTodos(e.target.checked);
                    setElegidos(new Set());
                  }}
                  className="h-4 w-4 rounded border-border accent-accent"
                />
                Ver también los que ya están resueltos
              </label>
            </div>

            <TableContainer>
              <Table>
                <THead>
                  <TR>
                    <TH className="w-8"> </TH>
                    <TH>Proveedor</TH>
                    <TH className="text-right">Facturas «contado»</TH>
                    <TH>Su ficha dice</TH>
                    <TH className="text-right">Trancadas hoy</TH>
                    <TH>Estado</TH>
                  </TR>
                </THead>
                <TBody>
                  {candidatos.map((c) => (
                    <FilaCandidato
                      key={c.proveedor_id}
                      c={c}
                      elegido={elegidos.has(c.proveedor_id)}
                      onAlternar={() => alternar(c.proveedor_id)}
                    />
                  ))}
                </TBody>
              </Table>
            </TableContainer>

            {/* COMPLETAR LA SEGUNDA CONDICIÓN. Marcar el convenio solo no desactiva el candado
                —hacen falta las dos— y 573 de 649 fichas dicen «Contado»: sin esto el botón
                terminaba avisando «pasalos a crédito» y mandando a hacerlo a mano, ficha por
                ficha. Aparece únicamente si hay elegidos a los que les cambiaría algo. */}
            {enContadoDeLaSeleccion > 0 && (
              <label className="flex cursor-pointer items-start gap-2 rounded-md border border-border bg-surface p-2">
                <input
                  type="checkbox"
                  className="mt-0.5 h-4 w-4 rounded border-border accent-accent"
                  checked={pasarACredito}
                  onChange={(e) => setPasarACredito(e.target.checked)}
                />
                <span className="text-xs">
                  <span className="font-medium text-content">
                    Pasar a «Crédito» {enContadoDeLaSeleccion} de los elegidos, que hoy dicen
                    «Contado»
                  </span>
                  <span className="block text-content-muted">
                    Hacen falta las dos cosas para que la regla se active: sin esto esos{" "}
                    {enContadoDeLaSeleccion} seguirían trancándose. Desmarcalo si alguno de verdad
                    se paga en el mostrador, porque ahí el candado corresponde. El plazo no se toca.
                  </span>
                </span>
              </label>
            )}

            <div className="flex flex-wrap items-center gap-2">
              <Button
                disabled={seleccionados.length === 0}
                loading={marcar.isPending || contar.isPending}
                onClick={marcarElegidos}
              >
                Creerle a la ficha en {seleccionados.length || ""} proveedor(es)
              </Button>
              <Button variant="ghost" onClick={() => setAbierto(false)}>
                Cerrar
              </Button>
              {seleccionados.length > 0 && (
                <span className="text-xs text-content-muted">
                  Cambia las facturas que entren de ahora en adelante.
                  {bloqueadasDeLaSeleccion > 0 &&
                    ` Las ${bloqueadasDeLaSeleccion} que ya están trancadas se ofrecen aparte.`}
                </span>
              )}
            </div>
          </>
        )}

        {/* Segundo paso: lo que YA está trancado. Se muestra la cifra antes de pedir la firma,
            porque firmar sobre una lista que no se vio es firmar en blanco. */}
        {porLiberar && (
          <div className="rounded-lg border border-pendiente/40 bg-pendiente/5 p-3">
            <p className="text-sm font-medium text-pendiente">
              🔒 Quedan {porLiberar.bloqueadas} factura(s) trancadas de esos proveedores, por{" "}
              {formatMoneda(porLiberar.montoCRC, "CRC")}
            </p>
            <p className="mt-0.5 text-xs text-content-muted">
              Lo que acabás de marcar vale para las facturas nuevas. Éstas llevan el candado escrito
              desde que entraron. Liberarlas es afirmar que no se pagaron por otra vía: queda
              guardado en cada una con tu nombre y la fecha.
            </p>
            {puedeLiberar ? (
              <div className="mt-3 space-y-3">
                <Input
                  label="Por qué estas facturas sí se pagan por banco"
                  value={motivo}
                  onChange={(e) => setMotivo(e.target.value)}
                  placeholder="Ej.: el emisor declara contado por costumbre; el convenio firmado es a 8 días."
                />
                <div className="flex flex-wrap items-center gap-2">
                  <Button
                    size="sm"
                    loading={liberar.isPending}
                    disabled={motivo.trim().length < MOTIVO_MINIMO}
                    onClick={liberarLoTrancado}
                  >
                    Liberar las {porLiberar.bloqueadas}
                  </Button>
                  <Button
                    size="sm"
                    variant="ghost"
                    onClick={() => {
                      setPorLiberar(null);
                      setMotivo("");
                    }}
                  >
                    Ahora no
                  </Button>
                  {motivo.trim().length > 0 && motivo.trim().length < MOTIVO_MINIMO && (
                    <span className="text-xs text-pendiente">
                      Faltan {MOTIVO_MINIMO - motivo.trim().length} caracteres del motivo.
                    </span>
                  )}
                </div>
              </div>
            ) : (
              <p className="mt-2 text-xs text-content-muted">
                Liberarlas requiere el permiso «Liberar una factura de contado para pagarla por
                banco». El convenio ya quedó marcado.
              </p>
            )}
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function FilaCandidato(props: { c: CandidatoConvenio; elegido: boolean; onAlternar: () => void }) {
  const { c } = props;
  return (
    <TR>
      <TD>
        <input
          type="checkbox"
          checked={props.elegido}
          onChange={props.onAlternar}
          aria-label={`Elegir ${c.nombre}`}
          className="h-4 w-4 rounded border-border accent-accent"
        />
      </TD>
      <TD className="font-medium">{c.nombre}</TD>
      <TD className="text-right tabular-nums">{formatMonto(String(c.facturas_contado))}</TD>
      <TD>
        {c.condicion_pago === "CREDITO" ? (
          <Badge tone="pendiente">Crédito</Badge>
        ) : (
          <Badge tone="neutral">Contado</Badge>
        )}
      </TD>
      <TD className="text-right tabular-nums">
        {c.bloqueadas > 0 ? (
          <span title={formatMoneda(c.monto_bloqueado, "CRC")}>
            {c.bloqueadas} · {formatMoneda(c.monto_bloqueado, "CRC")}
          </span>
        ) : (
          <span className="text-content-muted">—</span>
        )}
      </TD>
      <TD>
        {c.ya_listo ? (
          <Badge tone="positivo">resuelto</Badge>
        ) : c.falta_credito ? (
          // Marcado a medias: con la ficha en CONTADO la regla no se activa. Es el caso que más
          // confunde, porque la ficha «se ve configurada» y las facturas siguen trancándose.
          <span title="Tiene el convenio puesto, pero su ficha dice Contado: con eso la regla no se activa">
            <Badge tone="negativo">falta pasarlo a crédito</Badge>
          </span>
        ) : (
          <Badge tone="neutral">se le cree al comprobante</Badge>
        )}
      </TD>
    </TR>
  );
}
