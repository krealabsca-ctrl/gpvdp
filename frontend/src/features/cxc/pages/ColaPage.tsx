/**
 * CxC — Cola de cobro (/cxc/cola). El puesto de trabajo del gestor.
 *
 * Lo que hace distinta a esta pantalla: el orden. La cola se ordena por VALOR ESPERADO
 *
 *     vencido × probabilidad del tramo × factor de la forma de pago
 *
 * y no por antigüedad. Con los datos reales, un contrato de ₡14 929 con 5 días de mora vale
 * ₡13 436 esperados y otro de ₡2 917 con 186 días vale ₡146: ordenar por antigüedad gasta
 * el día en el peor caso.
 *
 * Dos reglas heredadas del resto del ERP:
 *   · el encabezado mide LO FILTRADO, no el universo;
 *   · los filtros y el orden se resuelven en el SERVIDOR (70 000 contratos).
 */

import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import {
  Badge,
  Button,
  EmptyState,
  ErrorState,
  Input,
  LoadingState,
  PageHeader,
  Select,
  Table,
  TableContainer,
  TBody,
  TD,
  TH,
  THead,
  TR,
  useToast,
} from "@/components/ui";
import type { BadgeTone } from "@/components/ui";
import { cn } from "@/lib/cn";
import { formatFecha, formatMoneda, hoyCR, toNumber } from "@/lib/format";
import { mensajeError } from "@/lib/apiError";
import { useTienePermiso } from "@/features/auth/permisos";
import { useCatalogosCxc, useCatalogosGestion, useCola, useRegistrarGestion } from "@/features/cxc/hooks";
import type { FilaCola } from "@/api/cxc";

const PAGE_SIZE = 50;

/** El tono sale del tramo, que es catálogo: la pantalla no define umbrales propios. */
function tonoTramo(codigo: string): BadgeTone {
  switch (codigo) {
    case "ADELANTADO":
    case "AL_DIA":
      return "positivo";
    case "PREVENTIVO":
    case "TEMPRANO":
      return "pendiente";
    case "MEDIO":
    case "TARDIO":
      return "negativo";
    default:
      return "negativo";
  }
}

export function ColaPage() {
  const tiene = useTienePermiso();
  const puedeGestionar = tiene("cxc.gestionar");
  const catalogos = useCatalogosCxc();

  const [qInput, setQInput] = useState("");
  const [q, setQ] = useState("");
  const [sedeId, setSedeId] = useState("");
  const [formaPagoId, setFormaPagoId] = useState("");
  const [asociacionId, setAsociacionId] = useState("");
  const [tramo, setTramo] = useState("");
  const [sinGestionar, setSinGestionar] = useState(false);
  const [promesaIncumplida, setPromesaIncumplida] = useState(false);
  const [tarjetaVencida, setTarjetaVencida] = useState(false);
  const [paraSuspender, setParaSuspender] = useState(false);
  const [morosa, setMorosa] = useState(false);
  const [arreglo, setArreglo] = useState("");
  const [orden, setOrden] = useState("");
  const [page, setPage] = useState(1);
  const [abierto, setAbierto] = useState("");

  useEffect(() => {
    const t = setTimeout(() => setQ(qInput.trim()), 300);
    return () => clearTimeout(t);
  }, [qInput]);
  useEffect(() => {
    setPage(1);
  }, [
    q,
    sedeId,
    formaPagoId,
    asociacionId,
    tramo,
    sinGestionar,
    promesaIncumplida,
    tarjetaVencida,
    paraSuspender,
    morosa,
    arreglo,
    orden,
  ]);

  const filtros = {
    ...(q ? { q } : {}),
    ...(sedeId ? { sede_id: sedeId } : {}),
    ...(formaPagoId ? { forma_pago_id: formaPagoId } : {}),
    ...(asociacionId ? { asociacion_id: asociacionId } : {}),
    ...(tramo ? { tramo } : {}),
    ...(sinGestionar ? { sin_gestionar: true } : {}),
    ...(promesaIncumplida ? { promesa_incumplida: true } : {}),
    ...(tarjetaVencida ? { tarjeta_vencida: true } : {}),
    ...(paraSuspender ? { para_suspender: true } : {}),
    ...(morosa ? { morosa: true } : {}),
    ...(arreglo ? { arreglo } : {}),
    ...(orden ? { orden } : {}),
  };
  const colaQ = useCola({ ...filtros, page, page_size: PAGE_SIZE });

  const cat = catalogos.data;
  const items = colaQ.data?.items ?? [];
  const total = colaQ.data?.total ?? 0;
  const r = colaQ.data?.resumen;
  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE));
  const opciones = (lista: { id: string; nombre: string; contratos: number }[] | undefined, todos: string) => [
    { value: "", label: todos },
    ...(lista ?? []).map((x) => ({
      value: x.id,
      label: x.contratos > 0 ? `${x.nombre} (${x.contratos.toLocaleString("es-CR")})` : x.nombre,
    })),
  ];

  return (
    <div className="flex flex-col gap-4">
      <PageHeader
        title="Cola de cobro"
        description="Ordenada por valor esperado: primero los contratos donde una gestión de hoy cambia el mes, no los más viejos."
      />

      {/* El encabezado mide LO FILTRADO. Si tocás un filtro, estas cifras se mueven. */}
      <div className="flex flex-wrap items-start gap-x-8 gap-y-3 rounded-xl border border-border bg-surface-raised px-4 py-3 shadow-card">
        <Cifra etiqueta="En la cola" valor={r ? r.contratos.toLocaleString("es-CR") : "—"} nota="contratos con algo vencido" />
        <Cifra etiqueta="Vencido" valor={r ? formatMoneda(r.vencido) : "—"} tono="negativo" nota="lo exigible hoy" />
        <Cifra
          etiqueta="Valor esperado"
          valor={r ? formatMoneda(r.valor_esperado) : "—"}
          tono="positivo"
          nota={r && toNumber(r.vencido) > 0 ? `${Math.round((toNumber(r.valor_esperado) / toNumber(r.vencido)) * 100)} % de lo vencido` : "recuperable"}
        />
        <Cifra etiqueta="Saldo total" valor={r ? formatMoneda(r.saldo) : "—"} nota="incluye lo no vencido" />
        <Cifra
          etiqueta="Sin gestionar"
          valor={r ? r.sin_gestionar.toLocaleString("es-CR") : "—"}
          nota="nadie los ha tocado"
        />
        <Cifra
          etiqueta="Promesas rotas"
          valor={r ? r.con_promesa_incumplida.toLocaleString("es-CR") : "—"}
          tono={r && r.con_promesa_incumplida > 0 ? "negativo" : undefined}
          nota={r ? `${r.con_promesa_vigente} vigentes` : undefined}
        />
        <Cifra
          etiqueta="Con arreglo"
          valor={r ? (r.arreglo_al_dia + r.arreglo_en_mora).toLocaleString("es-CR") : "—"}
          tono={r && r.arreglo_en_mora > 0 ? "negativo" : undefined}
          nota={r ? `${r.arreglo_al_dia} al día · ${r.arreglo_en_mora} en mora del plan` : undefined}
        />
        <Cifra
          etiqueta="Cartera morosa"
          valor={r ? r.cartera_morosa.toLocaleString("es-CR") : "—"}
          tono={r && r.cartera_morosa > 0 ? "negativo" : undefined}
          nota={r ? `${r.para_suspender} listos para suspender · ${r.suspendidos} ya cortados` : undefined}
        />
        {colaQ.isFetching && <span className="ml-auto self-center text-xs text-accent">actualizando…</span>}
      </div>

      {/* Lo que la cola deja fuera a propósito: que la exclusión sea visible. */}
      {r && r.por_vencer_contratos > 0 && (
        <p className="text-xs text-content-muted">
          Fuera de la cola a propósito:{" "}
          <span className="font-medium text-content">{r.por_vencer_contratos.toLocaleString("es-CR")} contratos</span> por{" "}
          <span className="font-medium text-content">{formatMoneda(r.por_vencer_monto)}</span> cuya cuota todavía no vence
          — el catálogo de tramos marca «Ninguno» como canal para los que están al día.
          {r.tarjetas_vencidas > 0 && (
            <>
              {" · "}
              <span className="text-negativo">{r.tarjetas_vencidas}</span> con tarjeta caducada: el débito no va a salir solo.
            </>
          )}
        </p>
      )}

      {/* Filtros */}
      <div className="flex flex-wrap items-end gap-3">
        <Input
          label="Buscar"
          value={qInput}
          onChange={(e) => setQInput(e.target.value)}
          placeholder="Contrato, cédula o cliente…"
          className="min-w-56"
        />
        <Select label="Sede" value={sedeId} onChange={(e) => setSedeId(e.target.value)} options={opciones(cat?.sedes, "Todas las sedes")} className="min-w-44" />
        <Select label="Forma de pago" value={formaPagoId} onChange={(e) => setFormaPagoId(e.target.value)} options={opciones(cat?.formas_pago, "Todas")} className="min-w-44" />
        <Select label="Asociación" value={asociacionId} onChange={(e) => setAsociacionId(e.target.value)} options={opciones(cat?.asociaciones, "Todas")} className="min-w-40" />
        <Select
          label="Tramo"
          value={tramo}
          onChange={(e) => setTramo(e.target.value)}
          options={[
            { value: "", label: "Todos los tramos" },
            ...(cat?.tramos ?? []).map((t) => ({ value: t.codigo, label: t.etiqueta })),
          ]}
          className="min-w-44"
        />
        <Select
          label="Ordenar por"
          value={orden}
          onChange={(e) => setOrden(e.target.value)}
          options={[
            { value: "", label: "Valor esperado (recomendado)" },
            { value: "saldo", label: "Saldo: mayor a menor" },
            { value: "mora", label: "Mora: más viejo primero" },
            { value: "sin_gestion", label: "Más olvidados primero" },
          ]}
          className="min-w-52"
        />
        <Select
          label="Arreglo de pago"
          value={arreglo}
          onChange={(e) => setArreglo(e.target.value)}
          options={[
            { value: "", label: "Con y sin arreglo" },
            { value: "AL_DIA", label: "Arreglo al día (no llamar)" },
            { value: "EN_MORA", label: "Arreglo en mora" },
            { value: "CON", label: "Con arreglo" },
            { value: "SIN", label: "Sin arreglo" },
          ]}
          className="min-w-48"
        />
        <Casilla marcado={sinGestionar} cambiar={setSinGestionar} etiqueta="Sin gestionar" />
        <Casilla marcado={promesaIncumplida} cambiar={setPromesaIncumplida} etiqueta="Promesa incumplida" />
        <Casilla marcado={tarjetaVencida} cambiar={setTarjetaVencida} etiqueta="Tarjeta vencida" />
        <Casilla marcado={paraSuspender} cambiar={setParaSuspender} etiqueta="Listos para suspender" />
        <Casilla marcado={morosa} cambiar={setMorosa} etiqueta="Cartera morosa" />
      </div>

      {colaQ.isPending ? (
        <LoadingState label="Armando la cola" />
      ) : colaQ.isError ? (
        <ErrorState message={mensajeError(colaQ.error)} onRetry={() => colaQ.refetch()} />
      ) : items.length === 0 ? (
        <EmptyState
          message={
            total === 0 && !q && !sinGestionar && !promesaIncumplida && !tarjetaVencida && !tramo
              ? "No hay nada vencido por cobrar. Si acabás de importar, generá los cargos primero."
              : "Ningún contrato coincide con estos filtros."
          }
        />
      ) : (
        <>
          <TableContainer>
            <Table>
              <THead>
                <TR>
                  <TH>Contrato</TH>
                  <TH>Contacto</TH>
                  <TH>Forma de pago</TH>
                  <TH className="text-right">Vencido</TH>
                  <TH>Mora</TH>
                  <TH className="text-right">Valor esperado</TH>
                  <TH>Última gestión</TH>
                  <TH />
                </TR>
              </THead>
              <TBody>
                {items.map((f) => (
                  <FilaDeCola
                    key={f.contrato_id}
                    f={f}
                    abierto={abierto === f.contrato_id}
                    alternar={() => setAbierto((a) => (a === f.contrato_id ? "" : f.contrato_id))}
                    puedeGestionar={puedeGestionar}
                  />
                ))}
              </TBody>
            </Table>
          </TableContainer>

          <div className="flex items-center justify-between text-sm text-content-muted">
            <span>
              {items.length} de {total.toLocaleString("es-CR")} contratos en la cola
            </span>
            {totalPages > 1 && (
              <div className="flex items-center gap-2">
                <Button size="sm" variant="secondary" disabled={page <= 1} onClick={() => setPage((p) => p - 1)}>
                  Anterior
                </Button>
                <span className="tabular-nums">
                  {page} / {totalPages}
                </span>
                <Button size="sm" variant="secondary" disabled={page >= totalPages} onClick={() => setPage((p) => p + 1)}>
                  Siguiente
                </Button>
              </div>
            )}
          </div>
        </>
      )}
    </div>
  );
}

/** Una fila de la cola; al abrirla aparece el panel para trabajar el contrato. */
function FilaDeCola({
  f,
  abierto,
  alternar,
  puedeGestionar,
}: {
  f: FilaCola;
  abierto: boolean;
  alternar: () => void;
  puedeGestionar: boolean;
}) {
  const telefonos = f.telefonos.split(/[,;]/).map((t) => t.trim()).filter(Boolean);
  return (
    <>
      <TR className={cn(abierto && "bg-surface-muted")}>
        <TD>
          <Link
            to={`/cxc/contratos/${encodeURIComponent(f.numero)}`}
            className="font-medium hover:text-accent"
            onClick={(e) => e.stopPropagation()}
          >
            {f.numero}
          </Link>
          <span className="block max-w-[13rem] truncate text-[11px] text-content-muted">{f.cliente || "—"}</span>
        </TD>
        <TD className="text-xs">
          {telefonos.length > 0 ? telefonos.slice(0, 2).join(" · ") : <span className="text-content-muted">sin teléfono</span>}
          {f.sede && <span className="block max-w-[12rem] truncate text-[11px] text-content-muted">{f.sede}</span>}
        </TD>
        <TD className="max-w-[12rem] text-xs">
          <span className="block truncate">{f.forma_pago || "—"}</span>
          {f.asociacion && <span className="block truncate text-[11px] text-content-muted">{f.asociacion}</span>}
          {f.tarjeta_vencida && (
            <Badge tone="negativo" className="mt-1">
              tarjeta vencida
            </Badge>
          )}
        </TD>
        <TD className="text-right font-medium tabular-nums text-negativo">
          {formatMoneda(f.vencido)}
          {toNumber(f.saldo) > toNumber(f.vencido) && (
            <span className="block text-[11px] font-normal text-content-muted">
              saldo {formatMoneda(f.saldo)}
            </span>
          )}
        </TD>
        <TD>
          <Badge tone={tonoTramo(f.tramo)}>{f.dias_mora} d</Badge>
          <span className="block text-[11px] text-content-muted">{f.tramo_etiqueta}</span>
          {/* Los MESES son la medida de la regla («18 meses o su equivalencia»): con un
              quincenal, 18 cuotas son 9 meses y el número de cuotas solo confundiría. */}
          {toNumber(f.meses_mora) > 0 && (
            <span className="block text-[11px] text-content-muted">
              {f.meses_mora} meses · {f.cuotas_vencidas} cuotas
            </span>
          )}
          {f.suspendido ? (
            <Badge tone="negativo" className="mt-1">
              servicio suspendido
            </Badge>
          ) : f.para_suspender ? (
            <Badge tone="negativo" className="mt-1">
              listo para suspender
            </Badge>
          ) : null}
          {f.en_cartera_morosa && !f.para_suspender && !f.suspendido && (
            <Badge tone="negativo" className="mt-1">
              cartera morosa
            </Badge>
          )}
        </TD>
        <TD className="text-right font-semibold tabular-nums text-positivo">{formatMoneda(f.valor_esperado)}</TD>
        <TD className="text-xs">
          {f.ultima_gestion ? (
            <>
              <span className="block">{f.ultimo_resultado}</span>
              <span className="block text-[11px] text-content-muted">
                {formatFecha(f.ultima_gestion)}
                {f.dias_sin_gestion !== null && f.dias_sin_gestion > 0 && ` · hace ${f.dias_sin_gestion} d`}
                {f.gestiones > 1 && ` · ${f.gestiones} gestiones`}
              </span>
            </>
          ) : (
            <span className="text-content-muted">nunca gestionado</span>
          )}
          {f.promesa_incumplida && (
            <Badge tone="negativo" className="mt-1">
              rompió la promesa del {formatFecha(f.promesa_fecha)}
            </Badge>
          )}
          {f.promesa_vigente && (
            <Badge tone="accent" className="mt-1">
              prometió el {formatFecha(f.promesa_fecha)}
            </Badge>
          )}
          {/* Un arreglo AL DÍA es razón para NO llamar hoy; uno EN MORA es lo contrario. */}
          {f.arreglo_estado === "AL_DIA" && (
            <Badge tone="positivo" className="mt-1">
              arreglo al día
            </Badge>
          )}
          {f.arreglo_estado === "EN_MORA" && (
            <Badge tone="negativo" className="mt-1">
              arreglo en mora
            </Badge>
          )}
          {f.arreglo_estado === "QUEBRADO" && (
            <Badge tone="negativo" className="mt-1">
              quebró el arreglo
            </Badge>
          )}
        </TD>
        <TD className="text-right">
          <Button size="sm" variant={abierto ? "primary" : "secondary"} onClick={alternar}>
            {abierto ? "Cerrar" : "Trabajar"}
          </Button>
        </TD>
      </TR>
      {abierto && (
        <TR className="bg-surface-muted">
          <TD colSpan={8}>
            <PanelDeGestion f={f} puedeGestionar={puedeGestionar} alCerrar={alternar} />
          </TD>
        </TR>
      )}
    </>
  );
}

/** El panel de trabajo: a quién llamar, qué decirle y qué quedó anotado. */
function PanelDeGestion({
  f,
  puedeGestionar,
  alCerrar,
}: {
  f: FilaCola;
  puedeGestionar: boolean;
  alCerrar: () => void;
}) {
  const toast = useToast();
  const catalogos = useCatalogosGestion();
  const registrar = useRegistrarGestion();

  const [canalId, setCanalId] = useState("");
  const [resultadoId, setResultadoId] = useState("");
  const [notas, setNotas] = useState("");
  const [promesaFecha, setPromesaFecha] = useState("");
  const [promesaMonto, setPromesaMonto] = useState("");

  const resultado = catalogos.data?.resultados.find((x) => x.id === resultadoId);
  const exigePromesa = resultado?.exige_promesa === true;

  // El canal por omisión es el que sugiere el tramo: el catálogo ya dice por dónde.
  useEffect(() => {
    if (canalId !== "" || !catalogos.data) return;
    const sugerido = catalogos.data.canales.find((c) =>
      f.canal_sugerido.toLowerCase().includes(c.nombre.toLowerCase()),
    );
    setCanalId(sugerido?.id ?? catalogos.data.canales[0]?.id ?? "");
  }, [catalogos.data, canalId, f.canal_sugerido]);

  const guardar = async () => {
    try {
      const res = await registrar.mutateAsync({
        contrato: f.numero,
        canal_id: canalId,
        resultado_id: resultadoId,
        ...(notas.trim() ? { notas: notas.trim() } : {}),
        ...(promesaFecha ? { promesa_fecha: promesaFecha } : {}),
        ...(promesaMonto.trim() ? { promesa_monto: promesaMonto.trim() } : {}),
      });
      toast.success(
        `Gestión anotada en ${f.numero}: ${res.resultado}` +
          (res.promesa_id ? ` · promesa para el ${formatFecha(promesaFecha)}` : ""),
      );
      alCerrar();
    } catch (e) {
      toast.error(mensajeError(e));
    }
  };

  return (
    <div className="flex flex-col gap-4 py-2">
      {/* Qué hacer con este contrato, según su tramo. Sale del catálogo. */}
      <div className="flex flex-wrap items-start gap-x-8 gap-y-2 text-xs">
        <div>
          <p className="text-[10.5px] font-semibold uppercase tracking-wide text-content-muted">Estrategia del tramo</p>
          <p className="text-sm">{f.estrategia || "—"}</p>
          <p className="text-[11px] text-content-muted">
            {f.tramo_etiqueta} · canal sugerido: {f.canal_sugerido || "—"}
          </p>
        </div>
        <div>
          <p className="text-[10.5px] font-semibold uppercase tracking-wide text-content-muted">Debe</p>
          <p className="text-sm font-semibold tabular-nums text-negativo">{formatMoneda(f.vencido)} vencidos</p>
          <p className="text-[11px] text-content-muted">
            {f.cargos_abiertos} {f.cargos_abiertos === 1 ? "cargo abierto" : "cargos abiertos"} · saldo {formatMoneda(f.saldo)}
          </p>
        </div>
        <div>
          <p className="text-[10.5px] font-semibold uppercase tracking-wide text-content-muted">Contacto</p>
          <p className="text-sm">{f.telefonos || "sin teléfono"}</p>
          <p className="max-w-[18rem] truncate text-[11px] text-content-muted">{f.correos || "sin correo"}</p>
        </div>
        {f.promesa_fecha && (
          <div>
            <p className="text-[10.5px] font-semibold uppercase tracking-wide text-content-muted">Última promesa</p>
            <p className={cn("text-sm", f.promesa_incumplida && "text-negativo")}>
              {formatFecha(f.promesa_fecha)} {f.promesa_monto && `por ${formatMoneda(f.promesa_monto)}`}
            </p>
            <p className="text-[11px] text-content-muted">
              {f.promesa_incumplida ? "no entró el pago" : "todavía en plazo"}
            </p>
          </div>
        )}
      </div>

      {!puedeGestionar ? (
        <p className="text-xs text-content-muted">
          No tenés permiso para anotar gestiones (<span className="font-mono">cxc.gestionar</span>). Podés ver la cola y
          abrir el contrato.
        </p>
      ) : catalogos.isPending ? (
        <LoadingState label="Cargando canales y resultados" />
      ) : catalogos.isError ? (
        <ErrorState message={mensajeError(catalogos.error)} onRetry={() => catalogos.refetch()} />
      ) : (
        <div className="flex flex-wrap items-end gap-3">
          <Select
            label="Canal"
            value={canalId}
            onChange={(e) => setCanalId(e.target.value)}
            options={(catalogos.data?.canales ?? []).map((c) => ({ value: c.id, label: c.nombre }))}
            className="min-w-36"
          />
          <Select
            label="Resultado"
            value={resultadoId}
            onChange={(e) => {
              setResultadoId(e.target.value);
              const r = catalogos.data?.resultados.find((x) => x.id === e.target.value);
              // Si el resultado ya no exige promesa, se limpia: no se guarda un compromiso
              // que el operador dejó por error de un resultado anterior.
              if (!r?.exige_promesa) {
                setPromesaFecha("");
                setPromesaMonto("");
              }
            }}
            options={[
              { value: "", label: "¿Qué pasó?" },
              ...(catalogos.data?.resultados ?? []).map((x) => ({ value: x.id, label: x.etiqueta })),
            ]}
            className="min-w-52"
          />
          {exigePromesa && (
            <>
              <Input
                label="Prometió pagar el"
                type="date"
                min={hoyCR()}
                value={promesaFecha}
                onChange={(e) => setPromesaFecha(e.target.value)}
                className="min-w-40"
              />
              <Input
                label="Monto prometido"
                value={promesaMonto}
                onChange={(e) => setPromesaMonto(e.target.value)}
                placeholder={f.vencido}
                className="min-w-36"
              />
            </>
          )}
          <label className="flex min-w-64 flex-1 flex-col gap-1">
            <span className="text-xs font-medium text-content-muted">Notas</span>
            <textarea
              rows={2}
              value={notas}
              onChange={(e) => setNotas(e.target.value)}
              placeholder="Qué dijo el cliente…"
              className="rounded-lg border border-border bg-surface px-3 py-2 text-sm text-content placeholder:text-content-muted focus:border-accent focus:outline-none"
            />
          </label>
          <Button
            onClick={guardar}
            disabled={resultadoId === "" || canalId === "" || (exigePromesa && promesaFecha === "") || registrar.isPending}
          >
            {registrar.isPending ? "Guardando…" : "Anotar gestión"}
          </Button>
        </div>
      )}
    </div>
  );
}

function Casilla({
  marcado,
  cambiar,
  etiqueta,
}: {
  marcado: boolean;
  cambiar: (v: boolean) => void;
  etiqueta: string;
}) {
  return (
    <label className="flex items-center gap-2 pb-2 text-xs text-content-muted">
      <input
        type="checkbox"
        checked={marcado}
        onChange={(e) => cambiar(e.target.checked)}
        className="h-3.5 w-3.5 rounded border-border accent-accent"
      />
      {etiqueta}
    </label>
  );
}

function Cifra({
  etiqueta,
  valor,
  nota,
  tono,
}: {
  etiqueta: string;
  valor: string;
  nota?: string;
  tono?: "negativo" | "positivo";
}) {
  return (
    <div className="min-w-[8.5rem]">
      <p className="text-[10.5px] font-semibold uppercase tracking-wide text-content-muted">{etiqueta}</p>
      <p
        className={cn(
          "text-lg font-semibold tabular-nums",
          tono === "negativo" ? "text-negativo" : tono === "positivo" ? "text-positivo" : "text-content",
        )}
      >
        {valor}
      </p>
      {nota && <p className="text-[10.5px] text-content-muted">{nota}</p>}
    </div>
  );
}
