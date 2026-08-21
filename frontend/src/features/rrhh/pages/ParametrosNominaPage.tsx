/**
 * RRHH — Parámetros de nómina (/rrhh/parametros). Versionados por año:
 * cargas sociales (obreras/patronales), tramos de renta, políticas del DF
 * (adelanto, redondeo, provisión) y el catálogo de conceptos.
 *
 * GUARDARRAÍL CCSS: los conceptos de sistema (salario, extras, comisiones,
 * bonos habituales…) se muestran con candado — el backend rechaza editarlos.
 * Un ingreso NO afecto a CCSS exige base legal.
 */

import { useState, type FormEvent } from "react";
import { createPortal } from "react-dom";
import {
  Badge,
  Button,
  Card,
  CardContent,
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
import { cn } from "@/lib/cn";
import { montoParaApi, toNumber } from "@/lib/format";

/**
 * Normaliza un porcentaje digitado ("10,66" → "10.66") para la API y los totales.
 * Los porcentajes NO pasan por montoParaApi (no llevan separador de miles).
 */
function pctPlano(s: string): string {
  return s.trim().replace(",", ".");
}
import { mensajeError } from "@/lib/apiError";
import { useTienePermiso } from "@/features/auth/permisos";
import {
  useConceptosNomina,
  useCrearConcepto,
  useDesactivarConcepto,
  useGuardarParametros,
  useParametrosNomina,
} from "@/features/rrhh/hooks";
import type { Carga, ConceptoInput, ConceptoNomina, ParametrosNomina, TramoRenta } from "@/api/rrhh";

const ANIO_BASE = 2026;

export function ParametrosNominaPage() {
  const tiene = useTienePermiso();
  const puedeEditar = tiene("rrhh.parametros");
  const [anio, setAnio] = useState(ANIO_BASE);
  const parametrosQ = useParametrosNomina(anio);

  return (
    <div className="flex flex-col gap-5">
      <PageHeader
        title="Parámetros de nómina"
        description="Cargas sociales, tramos de renta y políticas de la corrida — versionados por año. Los porcentajes legales se validan contra fuente primaria antes de producción."
        actions={
          <Select
            value={String(anio)}
            onChange={(e) => setAnio(Number(e.target.value))}
            options={[ANIO_BASE - 1, ANIO_BASE, ANIO_BASE + 1].map((a) => ({ value: String(a), label: String(a) }))}
          />
        }
      />

      {parametrosQ.isPending ? (
        <LoadingState label="Cargando parámetros" />
      ) : parametrosQ.isError ? (
        <ErrorState message={mensajeError(parametrosQ.error)} onRetry={() => parametrosQ.refetch()} />
      ) : (
        <ParametrosForm key={`${anio}-${parametrosQ.data.origen}`} anio={anio} inicial={parametrosQ.data} puedeEditar={puedeEditar} />
      )}

      <ConceptosCard puedeEditar={puedeEditar} />
    </div>
  );
}

// ---------------------------------------------------------------------------
// Parámetros del año (cargas + renta + políticas)
// ---------------------------------------------------------------------------

function ParametrosForm({ anio, inicial, puedeEditar }: { anio: number; inicial: ParametrosNomina; puedeEditar: boolean }) {
  const toast = useToast();
  const guardar = useGuardarParametros();

  const [cargas, setCargas] = useState<Carga[]>(inicial.cargas);
  const [tramos, setTramos] = useState<TramoRenta[]>(inicial.renta.tramos);
  const [creditoHijo, setCreditoHijo] = useState(inicial.renta.credito_hijo);
  const [creditoConyuge, setCreditoConyuge] = useState(inicial.renta.credito_conyuge);
  const [ins, setIns] = useState(inicial.ins_riesgos_pct);
  const [aplicaIna, setAplicaIna] = useState(inicial.aplica_ina);
  const [adelantoPct, setAdelantoPct] = useState(inicial.adelanto_pct);
  const [adelantoBase, setAdelantoBase] = useState(inicial.adelanto_base);
  const [redondeo, setRedondeo] = useState(inicial.redondeo);
  const [provisionBase, setProvisionBase] = useState(inicial.provision_base);
  const [aguinaldoPct, setAguinaldoPct] = useState(inicial.aguinaldo_pct);
  const [vacacionesPct, setVacacionesPct] = useState(inicial.vacaciones_pct);
  const [cesantiaPct, setCesantiaPct] = useState(inicial.cesantia_pct);

  const obreras = cargas.filter((c) => c.tipo === "OBRERO");
  const patronales = cargas.filter((c) => c.tipo === "PATRONAL");
  const totalObrero = obreras.reduce((s, c) => s + toNumber(pctPlano(c.pct)), 0);
  const totalPatronal = patronales.reduce((s, c) => s + toNumber(pctPlano(c.pct)), 0) + toNumber(pctPlano(ins));

  function setPct(codigo: string, pct: string) {
    setCargas((prev) => prev.map((c) => (c.codigo === codigo ? { ...c, pct } : c)));
  }

  function onGuardar(e: FormEvent) {
    e.preventDefault();
    // Normalización ANTES de enviar: los topes y créditos son montos (formato es-CR con
    // miles: "922.000" = ₡922 000, no ₡922) y los porcentajes aceptan coma decimal.
    const tramosApi = tramos.map((t) => ({
      hasta: t.hasta === null ? null : montoParaApi(t.hasta),
      pct: pctPlano(t.pct),
    }));
    if (tramosApi.some((t) => t.hasta === "")) {
      toast.error("Hay un tramo de renta con el límite vacío o ilegible.");
      return;
    }
    const creditoHijoApi = montoParaApi(creditoHijo);
    const creditoConyugeApi = montoParaApi(creditoConyuge);
    if (!creditoHijoApi || !creditoConyugeApi) {
      toast.error("Indicá los créditos fiscales por hijo y cónyuge (₡/mes).");
      return;
    }
    guardar.mutate(
      {
        anio,
        input: {
          cargas: cargas.map((c) => ({ ...c, pct: pctPlano(c.pct) })),
          renta: { tramos: tramosApi, credito_hijo: creditoHijoApi, credito_conyuge: creditoConyugeApi },
          ins_riesgos_pct: pctPlano(ins),
          aplica_ina: aplicaIna,
          adelanto_pct: pctPlano(adelantoPct),
          adelanto_base: adelantoBase,
          redondeo,
          provision_base: provisionBase,
          aguinaldo_pct: pctPlano(aguinaldoPct),
          vacaciones_pct: pctPlano(vacacionesPct),
          cesantia_pct: pctPlano(cesantiaPct),
        },
      },
      {
        onSuccess: () => toast.success(`Parámetros ${anio} guardados para la empresa.`),
        onError: (err) => toast.error(mensajeError(err)),
      },
    );
  }

  return (
    <form onSubmit={onGuardar} className="flex flex-col gap-4">
      {inicial.origen === "DEFAULT" && (
        <div className="rounded-lg border border-pendiente/30 bg-pendiente/10 px-3 py-2 text-sm text-content">
          📋 Estos son los <b>parámetros legales de referencia CR 2026</b> (verificados en la maqueta). Aún no se
          han guardado para esta empresa: revisalos con el Director Financiero y guardá para fijarlos al año {anio}.
        </div>
      )}

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <CargasCard
          titulo={`Deducciones del trabajador — ${totalObrero.toFixed(2)}%`}
          cargas={obreras}
          editable={puedeEditar}
          onPct={setPct}
        />
        <CargasCard
          titulo={`Cargas patronales — ${totalPatronal.toFixed(2)}% (incl. INS)`}
          cargas={patronales}
          editable={puedeEditar}
          onPct={setPct}
          extra={
            <div className="flex items-center justify-between gap-2 border-t border-border pt-2">
              <span className="text-sm text-content">INS Riesgos del Trabajo (variable por póliza)</span>
              <Input value={ins} onChange={(e) => setIns(e.target.value)} disabled={!puedeEditar} inputMode="decimal" className="w-24 text-right tabular-nums" />
            </div>
          }
        />
      </div>

      <Card>
        <CardContent className="flex flex-col gap-3">
          <h3 className="text-sm font-semibold text-content">Impuesto al salario (tramos mensuales)</h3>
          <TableContainer>
            <Table>
              <THead>
                <TR>
                  <TH>Tramo</TH>
                  <TH className="text-right">Hasta (₡ / mes)</TH>
                  <TH className="text-right">Tarifa %</TH>
                </TR>
              </THead>
              <TBody>
                {tramos.map((t, i) => (
                  <TR key={i}>
                    <TD className="text-xs text-content-muted">
                      {i === 0 ? "Exento" : t.hasta === null ? "Resto (abierto)" : `Tramo ${i}`}
                    </TD>
                    <TD className="text-right">
                      {t.hasta === null ? (
                        <span className="text-content-muted">∞</span>
                      ) : (
                        <Input
                          value={t.hasta}
                          onChange={(e) =>
                            setTramos((prev) => prev.map((x, j) => (j === i ? { ...x, hasta: e.target.value } : x)))
                          }
                          disabled={!puedeEditar}
                          inputMode="numeric"
                          className="ml-auto w-36 text-right tabular-nums"
                        />
                      )}
                    </TD>
                    <TD className="text-right">
                      <Input
                        value={t.pct}
                        onChange={(e) =>
                          setTramos((prev) => prev.map((x, j) => (j === i ? { ...x, pct: e.target.value } : x)))
                        }
                        disabled={!puedeEditar}
                        inputMode="decimal"
                        className="ml-auto w-20 text-right tabular-nums"
                      />
                    </TD>
                  </TR>
                ))}
              </TBody>
            </Table>
          </TableContainer>
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <Input label="Crédito fiscal por hijo (₡/mes)" value={creditoHijo} onChange={(e) => setCreditoHijo(e.target.value)} disabled={!puedeEditar} inputMode="numeric" className="text-right tabular-nums" />
            <Input label="Crédito fiscal por cónyuge (₡/mes)" value={creditoConyuge} onChange={(e) => setCreditoConyuge(e.target.value)} disabled={!puedeEditar} inputMode="numeric" className="text-right tabular-nums" />
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardContent className="flex flex-col gap-3">
          <h3 className="text-sm font-semibold text-content">Políticas de la corrida (decisiones del DF)</h3>
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
            <Input label="Adelanto quincenal (%)" value={adelantoPct} onChange={(e) => setAdelantoPct(e.target.value)} disabled={!puedeEditar} inputMode="decimal" className="text-right tabular-nums" />
            <Select
              label="Base del adelanto"
              value={adelantoBase}
              onChange={(e) => setAdelantoBase(e.target.value as typeof adelantoBase)}
              disabled={!puedeEditar}
              options={[
                { value: "SALARIO_BASE", label: "Salario base" },
                { value: "BRUTO", label: "Bruto estimado" },
              ]}
            />
            <Select
              label="INA aplica"
              value={aplicaIna ? "si" : "no"}
              onChange={(e) => setAplicaIna(e.target.value === "si")}
              disabled={!puedeEditar}
              options={[
                { value: "si", label: "Sí" },
                { value: "no", label: "No (menos de 5 empleados)" },
              ]}
            />
            <Select
              label="Redondeo"
              value={redondeo}
              onChange={(e) => setRedondeo(e.target.value as typeof redondeo)}
              disabled={!puedeEditar}
              options={[
                { value: "COLON", label: "Al colón" },
                { value: "CENTIMO", label: "Al céntimo" },
              ]}
            />
            <Select
              label="Base de provisiones (aguinaldo/cesantía)"
              value={provisionBase}
              onChange={(e) => setProvisionBase(e.target.value as typeof provisionBase)}
              disabled={!puedeEditar}
              options={[
                { value: "REMUNERACION_TOTAL", label: "Remuneración total" },
                { value: "SALARIO_BASE", label: "Solo salario base" },
              ]}
            />
            <Input label="Provisión aguinaldo (%)" value={aguinaldoPct} onChange={(e) => setAguinaldoPct(e.target.value)} disabled={!puedeEditar} inputMode="decimal" className="text-right tabular-nums" />
            <Input label="Provisión vacaciones (%)" value={vacacionesPct} onChange={(e) => setVacacionesPct(e.target.value)} disabled={!puedeEditar} inputMode="decimal" className="text-right tabular-nums" />
            <Input label="Provisión cesantía / FCL (%)" value={cesantiaPct} onChange={(e) => setCesantiaPct(e.target.value)} disabled={!puedeEditar} inputMode="decimal" className="text-right tabular-nums" />
          </div>
        </CardContent>
      </Card>

      {puedeEditar && (
        <div className="flex justify-end">
          <Button type="submit" loading={guardar.isPending}>
            Guardar parámetros {anio}
          </Button>
        </div>
      )}
    </form>
  );
}

function CargasCard({
  titulo,
  cargas,
  editable,
  onPct,
  extra,
}: {
  titulo: string;
  cargas: Carga[];
  editable: boolean;
  onPct: (codigo: string, pct: string) => void;
  extra?: React.ReactNode;
}) {
  return (
    <Card>
      <CardContent className="flex flex-col gap-2">
        <h3 className="text-sm font-semibold text-content">{titulo}</h3>
        {cargas.map((c) => (
          <div key={c.codigo} className="flex items-center justify-between gap-2">
            <span className="text-sm text-content">{c.nombre}</span>
            <Input
              value={c.pct}
              onChange={(e) => onPct(c.codigo, e.target.value)}
              disabled={!editable}
              inputMode="decimal"
              className="w-24 text-right tabular-nums"
            />
          </div>
        ))}
        {extra}
      </CardContent>
    </Card>
  );
}

// ---------------------------------------------------------------------------
// Conceptos de ingreso/deducción — guardarraíl CCSS con candado
// ---------------------------------------------------------------------------

function ConceptosCard({ puedeEditar }: { puedeEditar: boolean }) {
  const toast = useToast();
  const conceptosQ = useConceptosNomina();
  const desactivar = useDesactivarConcepto();
  const [nuevo, setNuevo] = useState(false);

  const items = conceptosQ.data ?? [];

  return (
    <Card>
      <CardContent className="flex flex-col gap-3">
        <div className="flex items-center justify-between">
          <div>
            <h3 className="text-sm font-semibold text-content">Conceptos de ingreso y deducción</h3>
            <p className="text-xs text-content-muted">
              🔒 Los conceptos de sistema están bloqueados por ley: comisiones y bonos habituales SON salario
              (base CCSS). Un ingreso no afecto a CCSS exige base legal.
            </p>
          </div>
          {puedeEditar && (
            <Button size="sm" onClick={() => setNuevo(true)}>
              Nuevo concepto
            </Button>
          )}
        </div>

        {conceptosQ.isPending ? (
          <LoadingState label="Cargando conceptos" />
        ) : (
          <TableContainer>
            <Table>
              <THead>
                <TR>
                  <TH>Concepto</TH>
                  <TH>Tipo</TH>
                  <TH className="text-center">CCSS</TH>
                  <TH className="text-center">Renta</TH>
                  <TH className="text-center">Aguinaldo</TH>
                  <TH>Base legal</TH>
                  {puedeEditar && <TH className="text-right">Acción</TH>}
                </TR>
              </THead>
              <TBody>
                {items.map((c) => (
                  <TR key={c.id} className={cn(!c.activo && "opacity-50")}>
                    <TD className="font-medium">
                      {c.de_sistema && <span title="Concepto de sistema — bloqueado por ley">🔒 </span>}
                      {c.nombre}
                    </TD>
                    <TD>
                      <Badge tone={c.tipo === "INGRESO" ? "positivo" : "neutral"}>{c.tipo}</Badge>
                    </TD>
                    <TD className="text-center">{marca(c, c.afecta_ccss)}</TD>
                    <TD className="text-center">{marca(c, c.afecta_renta)}</TD>
                    <TD className="text-center">{marca(c, c.afecta_aguinaldo)}</TD>
                    <TD className="max-w-64 truncate text-xs text-content-muted" title={c.base_legal}>
                      {c.base_legal || "—"}
                    </TD>
                    {puedeEditar && (
                      <TD className="text-right">
                        {!c.de_sistema && c.activo && (
                          <Button
                            size="sm"
                            variant="ghost"
                            className="text-negativo"
                            onClick={() =>
                              desactivar.mutate(c.id, {
                                onSuccess: () => toast.success("Concepto desactivado."),
                                onError: (err) => toast.error(mensajeError(err)),
                              })
                            }
                          >
                            Desactivar
                          </Button>
                        )}
                      </TD>
                    )}
                  </TR>
                ))}
              </TBody>
            </Table>
          </TableContainer>
        )}
      </CardContent>

      {nuevo && <ConceptoDialog onCerrar={() => setNuevo(false)} />}
    </Card>
  );
}

function marca(c: ConceptoNomina, v: boolean) {
  if (c.tipo === "DEDUCCION") return <span className="text-content-muted">—</span>;
  return v ? "✓" : <span className="text-content-muted">✗</span>;
}

function ConceptoDialog({ onCerrar }: { onCerrar: () => void }) {
  const toast = useToast();
  const crear = useCrearConcepto();

  const [nombre, setNombre] = useState("");
  const [tipo, setTipo] = useState<"INGRESO" | "DEDUCCION">("INGRESO");
  const [afectaCcss, setAfectaCcss] = useState(true);
  const [afectaRenta, setAfectaRenta] = useState(true);
  const [afectaAguinaldo, setAfectaAguinaldo] = useState(true);
  const [baseLegal, setBaseLegal] = useState("");

  const exigeBase = tipo === "INGRESO" && !afectaCcss;

  function onGuardar(e: FormEvent) {
    e.preventDefault();
    if (!nombre.trim()) return toast.error("Indicá el nombre del concepto.");
    if (exigeBase && !baseLegal.trim())
      return toast.error("Un ingreso no afecto a CCSS exige citar su base legal.");
    const input: ConceptoInput = {
      nombre: nombre.trim(),
      tipo,
      afecta_ccss: tipo === "INGRESO" ? afectaCcss : false,
      afecta_renta: tipo === "INGRESO" ? afectaRenta : false,
      afecta_aguinaldo: tipo === "INGRESO" ? afectaAguinaldo : false,
      base_legal: baseLegal.trim() || undefined,
    };
    crear.mutate(input, {
      onSuccess: () => {
        toast.success("Concepto creado.");
        onCerrar();
      },
      onError: (err) => toast.error(mensajeError(err)),
    });
  }

  const boolOpts = [
    { value: "si", label: "Sí" },
    { value: "no", label: "No" },
  ];

  return createPortal(
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4" onClick={onCerrar}>
      <div
        className="max-h-[90vh] w-full max-w-lg overflow-y-auto rounded-xl border border-border bg-surface-raised p-5 shadow-lifted"
        onClick={(e) => e.stopPropagation()}
      >
        <h2 className="text-base font-semibold text-content">Nuevo concepto</h2>
        <form onSubmit={onGuardar} className="mt-4 flex flex-col gap-3">
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <Input label="Nombre *" value={nombre} onChange={(e) => setNombre(e.target.value)} placeholder="Ej. Viáticos liquidados" />
            <Select
              label="Tipo"
              value={tipo}
              onChange={(e) => setTipo(e.target.value as typeof tipo)}
              options={[
                { value: "INGRESO", label: "Ingreso" },
                { value: "DEDUCCION", label: "Deducción" },
              ]}
            />
          </div>
          {tipo === "INGRESO" && (
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
              <Select label="Afecta CCSS" value={afectaCcss ? "si" : "no"} onChange={(e) => setAfectaCcss(e.target.value === "si")} options={boolOpts} />
              <Select label="Afecta renta" value={afectaRenta ? "si" : "no"} onChange={(e) => setAfectaRenta(e.target.value === "si")} options={boolOpts} />
              <Select label="Afecta aguinaldo" value={afectaAguinaldo ? "si" : "no"} onChange={(e) => setAfectaAguinaldo(e.target.value === "si")} options={boolOpts} />
            </div>
          )}
          <Input
            label={exigeBase ? "Base legal * (obligatoria: ingreso no afecto a CCSS)" : "Base legal"}
            value={baseLegal}
            onChange={(e) => setBaseLegal(e.target.value)}
            placeholder="Ej. Reglamento de gastos de viaje — no salarial con liquidación"
          />
          {exigeBase && (
            <p className="rounded-lg border border-pendiente/30 bg-pendiente/10 px-3 py-2 text-xs text-content">
              ⚖️ Marcaste un ingreso como <b>no afecto a CCSS</b>. Solo procede si es legítimamente no salarial
              (viáticos con liquidación, reembolsos). Lo salarial no se puede disfrazar: comisiones y bonos
              habituales son salario.
            </p>
          )}
          <div className="flex justify-end gap-2 pt-1">
            <Button type="button" variant="ghost" onClick={onCerrar}>
              Cancelar
            </Button>
            <Button type="submit" loading={crear.isPending}>
              Crear concepto
            </Button>
          </div>
        </form>
      </div>
    </div>,
    document.body,
  );
}
