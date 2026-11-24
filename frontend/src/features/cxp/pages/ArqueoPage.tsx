/**
 * El arqueo: lo que declaramos pagado y el banco nunca debitó.
 *
 * Hallazgo 3.1 de la auditoría Bancos↔CxP, el único CRÍTICO. Medido al construirlo: 62 facturas
 * por ₡2.757.710,84, con el estado de cuenta cargado hasta el 14 de setiembre, y CERO movimientos
 * enlazados de 31.195.
 *
 * ── POR QUÉ ESTA PANTALLA EXISTE ────────────────────────────────────────────────────────────
 *
 * Para un auditor, la diferencia entre «lo que el sistema dice que pagamos» y «lo que el banco
 * debitó» ES la partida conciliatoria de tesorería: la primera cifra que se pide en un arqueo.
 * Hasta hoy el sistema no la calculaba, no la mostraba y no la alertaba — y las dos lecturas
 * posibles de esas 62 facturas son incompatibles entre sí:
 *
 *   · El pago nunca se ejecutó → se siguen debiendo, y el proveedor lo sabe aunque nosotros no.
 *   · El pago se ejecutó y falló el reconocimiento → hay un defecto activo en el barrido.
 *
 * Nadie puede distinguirlas sin mirar caso por caso. Eso es lo que esta pantalla permite hacer.
 *
 * ── LAS TRES DECISIONES DE DISEÑO ───────────────────────────────────────────────────────────
 *
 * 1 · EL CORTE DEL BANCO VA PRIMERO Y SIEMPRE. Todo el reporte se lee contra esa fecha, y las
 *     facturas cuyo pago el banco todavía no alcanzó se cuentan APARTE. Sin eso, «0 excepciones»
 *     se leería como «todo cuadra» cuando lo que pasa es que falta cargar el estado de cuenta.
 *
 * 2 · PROPONE, NO DECIDE. Cada factura trae sus débitos candidatos —mismo monto, pocos días— y es
 *     una persona la que elige. Enlazar contra el movimiento equivocado deja dos errores donde
 *     había uno, y ninguno se vuelve a mirar porque los dos quedan «conciliados».
 *
 * 3 · LO QUE SE RECORTA SE DICE. La columna de débitos muestra 500 como mucho y sobre los datos
 *     reales son 2.388: el número verdadero va en pantalla. Un tope que no se declara se lee como
 *     «esto es todo», que es la peor forma de recortar.
 */

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, Link2, Link2Off, Search } from "lucide-react";

import { Badge, Button, Card, EmptyState, Input, PageHeader, Spinner, useToast } from "@/components/ui";
import { cxpApi, type FacturaSinRespaldo, type MovimientoCandidato } from "@/api/cxp";
import { bancosApi } from "@/api/bancos";
import { queryKeys } from "@/api/queryKeys";
import { useEmpresaId } from "@/features/bancos/useEmpresaId";
import { useRegistrarPagoParcial } from "@/features/cxp/hooks";
import { mensajeError } from "@/lib/apiError";
import { formatMoneda, toNumber } from "@/lib/format";

/** El mínimo que exige el servidor para cualquier motivo. Se refleja acá para avisar antes. */
const MOTIVO_MINIMO = 20;

export function ArqueoPage() {
  const empresaId = useEmpresaId();
  const toast = useToast();
  const qc = useQueryClient();
  const [abierta, setAbierta] = useState<string | null>(null);

  const arqueo = useQuery({
    queryKey: queryKeys.cxp.arqueo(empresaId),
    queryFn: () => cxpApi.excepcion(),
    enabled: !!empresaId,
  });

  const enlazar = useMutation({
    mutationFn: (v: { movimientoId: string; documentoId: string; motivo: string }) =>
      bancosApi.enlazarPagoAMano(v.movimientoId, v.documentoId, v.motivo),
    onSuccess: (r) => {
      toast.success(`Factura ${r.Consecutivo} conciliada contra su débito.`);
      setAbierta(null);
      // La raíz del módulo: la factura cambió de estado, así que la Bandeja y el tablero también
      // tienen que releer. La empresa va SEGUNDA en la clave o el prefijo no calza.
      void qc.invalidateQueries({ queryKey: queryKeys.cxp.raiz(empresaId) });
    },
    onError: (e) => toast.error(mensajeError(e)),
  });

  if (!empresaId) return null;
  if (arqueo.isLoading) {
    return (
      <div className="flex justify-center py-16">
        <Spinner />
      </div>
    );
  }
  if (arqueo.isError) {
    return <EmptyState message={"No se pudo cargar el arqueo: " + mensajeError(arqueo.error)} />;
  }
  const r = arqueo.data!;

  // Sin un solo movimiento cargado no hay contra qué comparar, y decir «todo cuadra» sería mentir.
  if (!r.corte_banco) {
    return (
      <>
        <Encabezado />
        <EmptyState message={"Todavía no hay estado de cuenta cargado. El arqueo compara lo que declaramos pagado contra lo que el banco debitó. Sin movimientos bancarios no hay contra qué compararlo: cargá primero el estado de cuenta en Bancos › Importador."} />
      </>
    );
  }

  return (
    <>
      <Encabezado />

      {/* El corte manda sobre todo lo de abajo, así que va arriba y no como nota al pie. */}
      <Card className="mb-4 border-l-4 border-l-accent p-4">
        <div className="flex flex-wrap items-baseline gap-x-6 gap-y-2 text-sm">
          <div>
            <span className="text-content-muted">Estado de cuenta cargado hasta</span>{" "}
            <b className="tabular-nums text-content">{r.corte_banco}</b>
          </div>
          <div>
            <span className="text-content-muted">Sin respaldo del banco</span>{" "}
            <b className="tabular-nums text-negativo">{r.sin_respaldo.length}</b>{" "}
            <span className="text-content-muted">por</span>{" "}
            <b className="tabular-nums text-negativo">{formatMoneda(r.total_sin_respaldo)}</b>
          </div>
          {r.aun_no_vencidas > 0 && (
            <div className="text-content-muted">
              {r.aun_no_vencidas} declarada{r.aun_no_vencidas === 1 ? "" : "s"} pagada
              {r.aun_no_vencidas === 1 ? "" : "s"} que el banco todavía no alcanzó —no son excepciones—
            </div>
          )}
        </div>
      </Card>

      {r.sin_respaldo.length === 0 ? (
        <EmptyState message={`Todo lo declarado pagado tiene su débito. Al ${r.corte_banco}, cada factura marcada como pagada tiene un movimiento bancario que la respalda.`} />
      ) : (
        <div className="space-y-2">
          {r.sin_respaldo.map((f) => (
            <FilaFactura
              key={f.documento_id}
              f={f}
              abierta={abierta === f.documento_id}
              onAbrir={() => setAbierta(abierta === f.documento_id ? null : f.documento_id)}
              onEnlazar={(movimientoId, motivo) =>
                enlazar.mutate({ movimientoId, documentoId: f.documento_id, motivo })
              }
              enviando={enlazar.isPending}
            />
          ))}
        </div>
      )}

      {/* La otra columna del arqueo. Va debajo y no al lado: en un teléfono dos columnas se
          vuelven ilegibles, y acá lo que importa es poder leer una fila entera. */}
      <h2 className="mb-2 mt-8 text-base font-semibold text-content">
        Débitos que no están aplicados a ninguna factura
      </h2>
      <p className="mb-3 text-xs text-content-muted">
        Del {r.desde} al {r.hasta}. La mayoría son planilla, traslados y comisiones, que nunca pasan
        por Cuentas por pagar.{" "}
        {r.debitos_encontrados > r.debitos_sin_factura.length && (
          <b className="text-content">
            Se muestran {r.debitos_sin_factura.length} de {r.debitos_encontrados}: acotá las fechas
            para ver el resto.
          </b>
        )}
      </p>
      <Card className="overflow-x-auto">
        <table className="w-full text-sm">
          <thead className="border-b border-border text-left text-xs uppercase tracking-wide text-content-muted">
            <tr>
              <th className="px-3 py-2">Fecha</th>
              <th className="px-3 py-2">Descripción</th>
              <th className="px-3 py-2">Cuenta</th>
              <th className="px-3 py-2">Partida</th>
              <th className="px-3 py-2 text-right">Monto</th>
            </tr>
          </thead>
          <tbody>
            {r.debitos_sin_factura.map((d) => (
              <tr key={d.movimiento_id} className="border-b border-border/50">
                <td className="whitespace-nowrap px-3 py-2 tabular-nums">{d.fecha}</td>
                <td className="px-3 py-2">{d.descripcion}</td>
                <td className="whitespace-nowrap px-3 py-2 text-content-muted">{d.cuenta}</td>
                <td className="whitespace-nowrap px-3 py-2 text-content-muted">
                  {d.clasificacion || "—"}
                </td>
                <td className="whitespace-nowrap px-3 py-2 text-right tabular-nums">
                  {formatMoneda(d.monto_crc)}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </Card>
    </>
  );
}

function Encabezado() {
  return (
    <PageHeader
      title="Arqueo de pagos"
      description="Lo que declaramos pagado y el banco nunca debitó, contra los débitos que no están aplicados a ninguna factura."
    />
  );
}

function FilaFactura({
  f,
  abierta,
  onAbrir,
  onEnlazar,
  enviando,
}: {
  f: FacturaSinRespaldo;
  abierta: boolean;
  onAbrir: () => void;
  onEnlazar: (movimientoId: string, motivo: string) => void;
  enviando: boolean;
}) {
  const [elegido, setElegido] = useState<string>("");
  const [motivo, setMotivo] = useState("");
  const elegidoEsExacto = f.candidatos.some((c) => c.movimiento_id === elegido && c.exacto);
  // Los dos grupos se separan acá y no en la consulta: el servidor devuelve los dos marcados, y
  // es la pantalla la que decide cuál muestra de entrada.
  const exactos = f.candidatos.filter((c) => c.exacto);
  const otros = f.candidatos.filter((c) => !c.exacto);
  const [verOtros, setVerOtros] = useState(false);

  // El código que hay que buscar en el estado de cuenta NO es el mismo según cómo se pagó: si el
  // pago salió agrupado, el banco del proveedor trae el de la REMESA y el de la factura no
  // aparece en ningún lado.
  const codigo = f.pagada_por_remesa ? f.referencia_remesa || f.huella : f.huella;

  return (
    <Card className="p-0">
      <button
        type="button"
        onClick={onAbrir}
        className="flex w-full flex-wrap items-center gap-x-4 gap-y-1 px-4 py-3 text-left hover:bg-surface-muted"
      >
        <div className="min-w-0 flex-1">
          <div className="truncate font-medium text-content">{f.proveedor}</div>
          <div className="truncate text-xs text-content-muted">
            {f.consecutivo} · código en el banco: <code className="font-mono">{codigo}</code>
            {f.pagada_por_remesa && " (pago agrupado)"}
          </div>
        </div>
        <div className="text-right">
          <div className="tabular-nums font-semibold text-content">
            {formatMoneda(f.neto_crc, f.moneda as "CRC" | "USD")}
          </div>
          <div className="text-xs text-content-muted">
            pago: {f.fecha_pago} ·{" "}
            <span className={f.dias_sin_respaldo > 30 ? "text-negativo" : ""}>
              {f.dias_sin_respaldo} días sin respaldo
            </span>
          </div>
        </div>
        {/*
          EL CONTEO ES SOLO DE LOS EXACTOS, Y ESO NO ES UN DETALLE.

          La consulta devuelve también los débitos cercanos de OTRO monto, porque son la puerta
          del pago parcial. Pero contarlos acá diría «8 candidatos» para una factura que no tiene
          ninguno: medido sobre los datos reales, son 465 sobre 62 facturas —casi todos débitos
          ajenos que cayeron cerca de la fecha— contra 31 que de verdad calzan.

          Un número inflado convierte la señal en ruido justo donde tiene que destacar, y empuja a
          emparejar lo primero que aparezca.
        */}
        {exactos.length > 0 ? (
          <Badge tone="pendiente">
            {exactos.length} candidato{exactos.length === 1 ? "" : "s"}
          </Badge>
        ) : (
          <Badge tone="neutral">sin candidato</Badge>
        )}
      </button>

      {abierta && (
        <div className="border-t border-border px-4 py-3">
          {exactos.length === 0 && !verOtros ? (
            <div className="space-y-2">
              <div className="flex items-start gap-2 text-sm text-content-muted">
                <Search className="mt-0.5 h-4 w-4 shrink-0" aria-hidden />
                <p>
                  Ningún débito del mismo monto cerca de esa fecha. Lo más probable es que{" "}
                  <b className="text-content">el pago nunca haya salido</b>: si el banco lo
                  rechazó, marcala como rebotada desde la Bandeja y volvé a incluirla en un corte
                  nuevo. También puede que el estado de cuenta de esa cuenta no esté cargado.
                </p>
              </div>
              {otros.length > 0 && (
                <Button variant="ghost" size="sm" onClick={() => setVerOtros(true)}>
                  ¿El banco pagó de menos? Ver {otros.length} débito{otros.length === 1 ? "" : "s"}{" "}
                  cercano{otros.length === 1 ? "" : "s"} de otro monto
                </Button>
              )}
            </div>
          ) : (
            <>
              <p className="mb-2 text-xs text-content-muted">
                {verOtros && exactos.length === 0 ? (
                  <>
                    Débitos cercanos de <b>otro monto</b>. Ninguno es evidencia de nada: se
                    muestran por si el banco pagó de menos, y solo sirve el que puedas verificar
                    contra el estado de cuenta.
                  </>
                ) : (
                  <>
                    Débitos del mismo monto cerca de la fecha de pago.{" "}
                    <b>Ninguno está confirmado</b>: elegí el que corresponda solo si lo
                    verificaste contra el estado de cuenta.
                  </>
                )}
              </p>
              <div className="mb-3 space-y-1">
                {(verOtros ? f.candidatos : exactos).map((c) => (
                  <Candidato
                    key={c.movimiento_id}
                    c={c}
                    elegido={elegido === c.movimiento_id}
                    onElegir={() => setElegido(c.movimiento_id)}
                  />
                ))}
              </div>
              {!verOtros && otros.length > 0 && (
                <Button variant="ghost" size="sm" className="mb-3" onClick={() => setVerOtros(true)}>
                  ¿El banco pagó de menos? Ver {otros.length} de otro monto
                </Button>
              )}

              {/*
                LOS DOS CAMINOS SE SEPARAN POR LO QUE DICE EL MONTO, no por lo que el usuario
                quiera hacer. Si el débito calza entero, es el pago: se enlaza. Si debitó menos,
                es un pago parcial y la factura sigue debiendo el resto (hallazgo 3.5).

                Ofrecer «enlazar» sobre un débito que no calza sería mentir: el servidor lo
                rechaza con las dos cifras, y el usuario habría escrito un motivo para nada.
              */}
              {elegidoEsExacto ? (
                <>
                  <Input
                    label="Por qué este débito es el pago de esta factura"
                    value={motivo}
                    onChange={(e) => setMotivo(e.target.value)}
                    placeholder="Ej.: el banco cortó la huella en la descripción; verificado contra el estado de cuenta"
                  />
                  <div className="mt-2 flex items-center justify-between gap-3">
                    <span className="text-[11px] text-content-muted">
                      {motivo.trim().length < MOTIVO_MINIMO
                        ? `Faltan ${MOTIVO_MINIMO - motivo.trim().length} caracteres: dentro de seis meses esta nota es lo único que explica el emparejamiento.`
                        : "Queda registrado quién lo enlazó y por qué."}
                    </span>
                    <Button
                      onClick={() => onEnlazar(elegido, motivo)}
                      loading={enviando}
                      disabled={!elegido || motivo.trim().length < MOTIVO_MINIMO}
                    >
                      <Link2 className="mr-1.5 h-4 w-4" aria-hidden />
                      Enlazar
                    </Button>
                  </div>
                </>
              ) : elegido ? (
                <ParcialDesdeElArqueo
                  f={f}
                  candidato={f.candidatos.find((c) => c.movimiento_id === elegido)!}
                />
              ) : null}
            </>
          )}
        </div>
      )}
    </Card>
  );
}

function Candidato({
  c,
  elegido,
  onElegir,
}: {
  c: MovimientoCandidato;
  elegido: boolean;
  onElegir: () => void;
}) {
  const dias = c.dias_de_diferencia;
  return (
    <button
      type="button"
      onClick={onElegir}
      className={
        "flex w-full flex-wrap items-center gap-x-3 gap-y-1 rounded-lg border px-3 py-2 text-left text-sm " +
        (elegido ? "border-accent bg-accent/5" : "border-border hover:bg-surface-muted")
      }
    >
      <span className="tabular-nums text-content-muted">{c.fecha}</span>
      <span className="min-w-0 flex-1 truncate text-content">{c.descripcion}</span>
      <span className="text-xs text-content-muted">{c.cuenta}</span>
      <span className="text-xs text-content-muted">
        {dias === 0 ? "el mismo día" : dias > 0 ? `${dias} d después` : `${-dias} d antes`}
      </span>
      <span className="tabular-nums font-medium text-content">{formatMoneda(c.monto_crc)}</span>
      {/* La etiqueta NO es decoración: es lo que decide si el botón de abajo dice «Enlazar» o
          «Registrar pago parcial». Sin ella, dos líneas con montos distintos se ven iguales. */}
      {c.exacto ? (
        <Badge tone="positivo">calza exacto</Badge>
      ) : (
        <Badge tone="pendiente">monto distinto</Badge>
      )}
    </button>
  );
}

/** Deshacer un enlace: existe para que emparejar mal no sea un camino de ida. */
export function DesenlazarBoton({ movimientoId, onHecho }: { movimientoId: string; onHecho: () => void }) {
  const toast = useToast();
  const [motivo, setMotivo] = useState("");
  const [abierto, setAbierto] = useState(false);
  const m = useMutation({
    mutationFn: () => bancosApi.desenlazarPago(movimientoId, motivo),
    onSuccess: (r) => {
      toast.success(
        `Se deshizo la conciliación: ${r.facturas_reabiertas} factura${r.facturas_reabiertas === 1 ? "" : "s"} vuelve${r.facturas_reabiertas === 1 ? "" : "n"} a esperar su débito.`,
      );
      setAbierto(false);
      onHecho();
    },
    onError: (e) => toast.error(mensajeError(e)),
  });

  if (!abierto) {
    return (
      <Button variant="secondary" onClick={() => setAbierto(true)}>
        <Link2Off className="mr-1.5 h-4 w-4" aria-hidden />
        Deshacer enlace
      </Button>
    );
  }
  return (
    <div className="space-y-2 rounded-lg border border-pendiente/40 bg-pendiente/5 p-3">
      <div className="flex items-start gap-2 text-xs text-content">
        <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-pendiente" aria-hidden />
        <p>
          La factura vuelve a <b>PAGADO</b> y reaparece en el arqueo esperando su débito. No vuelve a
          la cola de pago: lo que se retira es la confirmación del banco, no el pago declarado.
        </p>
      </div>
      <Input
        label="Por qué este débito no era el pago de esa factura"
        value={motivo}
        onChange={(e) => setMotivo(e.target.value)}
      />
      <div className="flex justify-end gap-2">
        <Button variant="secondary" onClick={() => setAbierto(false)}>
          Cancelar
        </Button>
        <Button
          onClick={() => m.mutate()}
          loading={m.isPending}
          disabled={motivo.trim().length < MOTIVO_MINIMO}
        >
          Deshacer
        </Button>
      </div>
    </div>
  );
}

/**
 * Registrar un pago parcial desde el arqueo (hallazgo 3.5).
 *
 * Aparece cuando el débito elegido NO calza con el neto. El monto viene precargado con lo que el
 * banco debitó: es el caso normal —entró eso— y escribirlo de nuevo solo agrega una forma de
 * equivocarse. Se puede bajar si ese débito cubre además otra factura.
 *
 * Lo que la pantalla deja claro, porque es lo que no se ve solo: la factura NO queda saldada. Se
 * registra lo que entró, el saldo queda a la vista, y volver a cobrarlo es una decisión aparte
 * —si la diferencia era una nota de crédito, cobrarla le paga de más al proveedor—.
 */
function ParcialDesdeElArqueo({
  f,
  candidato,
}: {
  f: FacturaSinRespaldo;
  candidato: MovimientoCandidato;
}) {
  const toast = useToast();
  const qc = useQueryClient();
  const empresaId = useEmpresaId();
  const registrar = useRegistrarPagoParcial();
  const [monto, setMonto] = useState(candidato.monto_crc);
  const [motivo, setMotivo] = useState("");

  const moneda = f.moneda as "CRC" | "USD";
  const neto = toNumber(f.neto_crc);
  const aplicar = toNumber(monto);
  const quedaria = Math.max(0, neto - aplicar);

  return (
    <div className="space-y-2 rounded-lg border border-pendiente/40 bg-pendiente/5 p-3">
      <div className="flex items-start gap-2 text-xs text-content">
        <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-pendiente" aria-hidden />
        <p>
          Ese débito es de <b>{formatMoneda(candidato.monto_crc, moneda)}</b> y la factura espera{" "}
          <b>{formatMoneda(f.neto_crc, moneda)}</b>. Si el banco pagó de menos, se registra lo que
          entró y la factura <b>sigue debiendo el resto</b>.
        </p>
      </div>
      <div className="flex flex-wrap items-end gap-3">
        <Input
          label="Cuánto de ese débito es de esta factura"
          value={monto}
          onChange={(e) => setMonto(e.target.value)}
          className="w-44"
        />
        <span className="pb-2 text-xs text-content-muted">
          quedaría debiendo <b className="text-content">{formatMoneda(String(quedaria), moneda)}</b>
        </span>
      </div>
      <Input
        label="Por qué el banco debitó menos"
        value={motivo}
        onChange={(e) => setMotivo(e.target.value)}
        placeholder="Ej.: el banco rechazó una línea del corte y entró solo una parte"
      />
      <div className="flex items-center justify-between gap-3">
        <span className="text-[11px] text-content-muted">
          {motivo.trim().length < MOTIVO_MINIMO
            ? `Faltan ${MOTIVO_MINIMO - motivo.trim().length} caracteres: el saldo que queda hay que poder explicarlo.`
            : "El saldo NO vuelve solo a la cola de pago: se decide aparte."}
        </span>
        <Button
          loading={registrar.isPending}
          disabled={!monto.trim() || motivo.trim().length < MOTIVO_MINIMO}
          onClick={() =>
            registrar.mutate(
              { id: f.documento_id, movimientoId: candidato.movimiento_id, monto, motivo },
              {
                onSuccess: (s) => {
                  toast.success(
                    toNumber(s.saldo) === 0
                      ? "Con esto la factura quedó saldada y conciliada."
                      : `Registrado. La factura sigue debiendo ${formatMoneda(s.saldo, moneda)}.`,
                  );
                  void qc.invalidateQueries({ queryKey: queryKeys.cxp.arqueo(empresaId) });
                },
                onError: (e) => toast.error(mensajeError(e)),
              },
            )
          }
        >
          Registrar pago parcial
        </Button>
      </div>
    </div>
  );
}
