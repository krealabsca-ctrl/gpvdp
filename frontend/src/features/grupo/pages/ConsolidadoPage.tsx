/**
 * Grupo — Consolidado (/grupo).
 *
 * Una VISTA: suma lo que el usuario ya podía ver empresa por empresa y no escribe nada. Por eso no
 * tiene un solo botón de acción; para operar están las pantallas de cada empresa.
 *
 * La pantalla está construida alrededor de lo que el número NO dice, porque es lo que hace que un
 * consolidado sirva o engañe:
 *  - si falta una empresa (por acceso) lo dice arriba, no al pie;
 *  - si una empresa no llega al 90 % clasificado, se marca en su fila y se explica;
 *  - lo intercompañía se informa aparte, sin restarlo (decisión del Director Financiero).
 */

import {
  Badge,
  Card,
  CardContent,
  ErrorState,
  LoadingState,
  PageHeader,
  TBody,
  TD,
  TH,
  THead,
  TR,
  Table,
  TableContainer,
} from "@/components/ui";
import { cn } from "@/lib/cn";
import { etiquetaPeriodo, formatMoneda, toNumber } from "@/lib/format";
import { esStatus, mensajeError } from "@/lib/apiError";
import { usePeriodoActivo } from "@/app/PeriodoProvider";
import { useResumenGrupo } from "@/features/grupo/hooks";
import type {
  FilaEmpresaGrupo,
  NaturalezaGrupo,
  PartidaGrupo,
  ResumenGrupo,
} from "@/api/grupo";

export function ConsolidadoPage() {
  // El período es contexto ambiente del ERP: un único selector en la barra superior.
  const { periodo } = usePeriodoActivo();
  const q = useResumenGrupo(periodo);

  return (
    <div className="flex flex-col gap-5">
      <PageHeader
        title="Consolidado del grupo"
        description={`${etiquetaPeriodo(periodo)} · las empresas que podés ver, sumadas. Solo lectura.`}
      />

      {q.isPending ? (
        <LoadingState label="Sumando las empresas del grupo" />
      ) : q.isError ? (
        // Sin acceso de lectura a ninguna empresa el servidor responde 403 con el motivo, y ahí
        // «Reintentar» no ayuda: el permiso no va a aparecer por volver a pedirlo. Ofrecerlo hace
        // creer que la falla es pasajera y manda a la persona a insistir en vez de a pedir el acceso.
        <ErrorState
          message={mensajeError(q.error)}
          onRetry={esStatus(q.error, 403) ? undefined : () => q.refetch()}
        />
      ) : (
        <Consolidado r={q.data} />
      )}
    </div>
  );
}

function Consolidado({ r }: { r: ResumenGrupo }) {
  return (
    <div className="flex flex-col gap-5">
      <AvisoDelGrupo r={r} />

      <Totales r={r} />
      <PorEmpresa filas={r.empresas} />
      <EntreEmpresas r={r} />
      <Partidas partidas={r.partidas} />
    </div>
  );
}

/**
 * El aviso va ARRIBA de los números, no debajo.
 *
 * Un total que omite una empresa se lee como el total del grupo; si la advertencia está al pie,
 * quien mira el KPI nunca la ve.
 *
 * La frase del servidor viene sin montos (excluidas, empresas vacías, empresas flojas) y acá se le
 * suma lo intercompañía con el formato de plata del ERP. El servidor no formatea dinero: cuando lo
 * hacía, el aviso decía «₡1290000» justo arriba de una tabla que decía «₡1 290 000,00».
 */
function AvisoDelGrupo({ r }: { r: ResumenGrupo }) {
  const interno = toNumber(r.entre_empresas_ebitda_crc);
  const frases: string[] = [];
  if (r.aviso) frases.push(r.aviso.replace(/\.$/, ""));
  if (interno > 0) {
    frases.push(
      `de los totales, ${formatMoneda(r.entre_empresas_ebitda_crc)} son operaciones entre empresas del grupo que sí afectan el resultado (no se eliminan: se informan)`,
    );
  }
  if (frases.length === 0) return null;

  return (
    <div
      className={cn(
        "rounded-lg border px-4 py-3 text-sm",
        r.completo
          ? "border-accent/30 bg-accent/5 text-content"
          : "border-pendiente/40 bg-pendiente/5 text-content",
      )}
    >
      <span className="font-medium">Leé esto antes del número: </span>
      {frases.join(" · ")}.
    </div>
  );
}

function Totales({ r }: { r: ResumenGrupo }) {
  const empresas = r.empresas.length;
  return (
    <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
      <Kpi
        titulo="Ingresos del grupo"
        monto={r.ingresos_crc}
        nota={`${empresas} de ${r.empresas_del_sistema} empresa${r.empresas_del_sistema === 1 ? "" : "s"}`}
        tono="positivo"
      />
      <Kpi titulo="Gastos del grupo" monto={r.gastos_crc} nota={`${r.movimientos.toLocaleString("es-CR")} movimientos`} tono="negativo" />
      <Kpi
        titulo="EBITDA"
        monto={r.ebitda_crc}
        nota="Ingresos − gastos, sin los neutros"
        tono={toNumber(r.ebitda_crc) >= 0 ? "positivo" : "negativo"}
        destacado
      />
      {/* El neutro es el número MÁS GRANDE de este grupo (traslados, ahorro, reservas). Omitirlo
          hace pensar que falta plata; mostrarlo como ingreso o gasto infla el resultado. */}
      <Kpi titulo="Movido sin ser ingreso ni gasto" monto={r.neutro_crc} nota="Traslados, ahorro y reservas" tono="neutral" />
    </div>
  );
}

function Kpi({
  titulo,
  monto,
  nota,
  tono,
  destacado,
}: {
  titulo: string;
  monto: string;
  nota: string;
  tono: "positivo" | "negativo" | "neutral";
  destacado?: boolean;
}) {
  const color =
    tono === "positivo" ? "text-positivo" : tono === "negativo" ? "text-negativo" : "text-content";
  return (
    <Card className={cn(destacado && "border-accent/40")}>
      <CardContent className="py-4">
        <p className="text-xs font-medium uppercase tracking-wide text-content-muted">{titulo}</p>
        <p className={cn("mt-1 text-2xl font-semibold tabular-nums", color)}>{formatMoneda(monto)}</p>
        <p className="mt-1 text-xs text-content-muted">{nota}</p>
      </CardContent>
    </Card>
  );
}

function PorEmpresa({ filas }: { filas: FilaEmpresaGrupo[] }) {
  return (
    <section className="flex flex-col gap-2">
      <h2 className="text-sm font-semibold text-content">Quién aporta qué</h2>
      <TableContainer>
        <Table>
          <THead>
            <TR>
              <TH>Empresa</TH>
              <TH className="text-right">Ingresos</TH>
              <TH className="text-right">Gastos</TH>
              <TH className="text-right">EBITDA</TH>
              <TH className="text-right">Neutro</TH>
              <TH className="text-right">Movs.</TH>
              <TH className="text-right">Clasificado</TH>
            </TR>
          </THead>
          <TBody>
            {filas.map((f) => (
              <TR key={f.empresa_id}>
                <TD className="font-medium">{f.empresa}</TD>
                <TD className="text-right tabular-nums">{formatMoneda(f.ingresos_crc)}</TD>
                <TD className="text-right tabular-nums">{formatMoneda(f.gastos_crc)}</TD>
                <TD
                  className={cn(
                    "text-right font-medium tabular-nums",
                    toNumber(f.ebitda_crc) >= 0 ? "text-positivo" : "text-negativo",
                  )}
                >
                  {formatMoneda(f.ebitda_crc)}
                </TD>
                <TD className="text-right tabular-nums text-content-muted">{formatMoneda(f.neutro_crc)}</TD>
                <TD className="text-right tabular-nums text-content-muted">
                  {f.movimientos.toLocaleString("es-CR")}
                </TD>
                <TD className="text-right">
                  {/* Sin movimientos NO es «100 % clasificado»: el porcentaje de un conjunto vacío da
                      100 y se leería como un mes impecable. Y cuando sí hay datos, el % va con el
                      monto sin partida: «97 %» tranquiliza, «97 % · ₡4,7M sin partida» es la verdad
                      que hace falta para decidir si el EBITDA sirve. */}
                  {f.sin_datos ? (
                    <Badge tone="neutral">sin movimientos</Badge>
                  ) : (
                    <div className="flex flex-col items-end gap-0.5">
                      <Badge tone={f.confiable ? "positivo" : "pendiente"}>{f.pct_clasificado} %</Badge>
                      {toNumber(f.sin_clasificar_crc) > 0 && (
                        <span className="text-xs tabular-nums text-content-muted">
                          {formatMoneda(f.sin_clasificar_crc)} sin partida
                        </span>
                      )}
                    </div>
                  )}
                </TD>
              </TR>
            ))}
          </TBody>
        </Table>
      </TableContainer>
    </section>
  );
}

/**
 * Operaciones entre empresas del grupo.
 *
 * Se detectan por el NOMBRE de la partida, y la pantalla lo dice: hoy no existe una marca formal de
 * «esto es intercompañía», así que esto es lo que se pudo identificar, no la cifra cerrada. Presentar
 * una heurística como un dato exacto es peor que no mostrarla.
 */
function EntreEmpresas({ r }: { r: ResumenGrupo }) {
  if (r.entre_empresas.length === 0) return null;
  return (
    <section className="flex flex-col gap-2">
      <div>
        <h2 className="text-sm font-semibold text-content">Entre empresas del grupo</h2>
        <p className="mt-0.5 text-xs text-content-muted">
          {formatMoneda(r.entre_empresas_crc)} en total, de los cuales{" "}
          <strong className="font-medium text-content">{formatMoneda(r.entre_empresas_ebitda_crc)}</strong>{" "}
          afectan el resultado. No se restan de los totales: se informan. Identificadas por el nombre
          de la partida, así que puede faltar alguna.
        </p>
      </div>
      <TableContainer>
        <Table>
          <THead>
            <TR>
              <TH>Empresa</TH>
              <TH>Contraparte</TH>
              <TH>Partida</TH>
              <TH className="text-right">Movs.</TH>
              <TH className="text-right">Monto</TH>
              <TH>Efecto</TH>
            </TR>
          </THead>
          <TBody>
            {r.entre_empresas.map((o, i) => (
              <TR key={`${o.empresa_id}-${o.partida}-${i}`}>
                <TD className="font-medium">{o.empresa}</TD>
                <TD>{o.contraparte}</TD>
                <TD className="text-content-muted">{o.partida}</TD>
                <TD className="text-right tabular-nums text-content-muted">{o.movimientos}</TD>
                <TD className="text-right tabular-nums">{formatMoneda(o.monto_crc)}</TD>
                <TD>
                  <Badge tone={o.afecta_ebitda ? "pendiente" : "neutral"}>
                    {o.afecta_ebitda ? `${etiquetaNaturaleza(o.naturaleza)} · toca el EBITDA` : "neutro"}
                  </Badge>
                </TD>
              </TR>
            ))}
          </TBody>
        </Table>
      </TableContainer>
    </section>
  );
}

/**
 * Partidas más grandes del grupo, con el desglose de quién las genera.
 *
 * El desglose es el punto: una partida grande puede ser un problema de todos o de una sola empresa,
 * y sin abrirla no se distingue. La barra es CSS, no una librería de gráficos: son tres empresas y
 * lo que se necesita es la proporción, no un eje.
 */
function Partidas({ partidas }: { partidas: PartidaGrupo[] }) {
  if (partidas.length === 0) return null;
  return (
    <section className="flex flex-col gap-2">
      <div>
        <h2 className="text-sm font-semibold text-content">Las partidas más grandes del grupo</h2>
        <p className="mt-0.5 text-xs text-content-muted">
          Los neutros quedan afuera: son traslados y ahorro, no el ingreso ni el gasto del grupo.
        </p>
      </div>
      <TableContainer>
        <Table>
          <THead>
            <TR>
              <TH>Partida</TH>
              <TH className="text-right">Monto</TH>
              <TH className="w-2/5">De dónde sale</TH>
            </TR>
          </THead>
          <TBody>
            {partidas.map((p, i) => (
              <TR key={`${p.concepto}-${p.partida}-${i}`}>
                <TD>
                  <div className="flex flex-col">
                    <span className="font-medium">{p.partida}</span>
                    <span className="text-xs text-content-muted">
                      {p.concepto} · {etiquetaNaturaleza(p.naturaleza)} · {p.movimientos} mov.
                    </span>
                  </div>
                </TD>
                <TD
                  className={cn(
                    "text-right font-medium tabular-nums",
                    p.naturaleza === "INGRESO" ? "text-positivo" : "text-negativo",
                  )}
                >
                  {formatMoneda(p.monto_crc)}
                </TD>
                <TD>
                  <Reparto partida={p} />
                </TD>
              </TR>
            ))}
          </TBody>
        </Table>
      </TableContainer>
    </section>
  );
}

/** Proporción por empresa: barra + los nombres, porque una barra sin etiqueta no dice de quién es. */
function Reparto({ partida }: { partida: PartidaGrupo }) {
  const total = partida.por_empresa.reduce((acc, e) => acc + Math.abs(toNumber(e.monto_crc)), 0);
  if (total <= 0) return <span className="text-xs text-content-muted">—</span>;

  const tono = partida.naturaleza === "INGRESO" ? "bg-positivo" : "bg-negativo";
  return (
    <div className="flex flex-col gap-1">
      <div className="flex h-2 overflow-hidden rounded-full bg-surface-muted" aria-hidden="true">
        {partida.por_empresa.map((e, i) => (
          <div
            key={e.empresa}
            className={cn(tono, i % 2 === 1 && "opacity-60", i % 3 === 2 && "opacity-35")}
            style={{ width: `${(Math.abs(toNumber(e.monto_crc)) / total) * 100}%` }}
          />
        ))}
      </div>
      <div className="flex flex-wrap gap-x-3 gap-y-0.5 text-xs text-content-muted">
        {partida.por_empresa.map((e) => (
          <span key={e.empresa} className="tabular-nums">
            {e.empresa} {Math.round((Math.abs(toNumber(e.monto_crc)) / total) * 100)} %
          </span>
        ))}
      </div>
    </div>
  );
}

const ETIQUETA_NATURALEZA: Record<NaturalezaGrupo, string> = {
  INGRESO: "ingreso",
  GASTO: "gasto",
  NEUTRO: "neutro",
  SIN_CLASIFICAR: "sin clasificar",
};

function etiquetaNaturaleza(n: NaturalezaGrupo): string {
  return ETIQUETA_NATURALEZA[n] ?? n;
}
