/**
 * «Qué ve» una persona: el alcance de «Mi presupuesto» y «Mi ingreso», desde su ficha.
 *
 * EL PEDIDO (Director Financiero, 16-nov-2026): «¿cómo hago para que ellos puedan ver esa parte,
 * desde dónde les doy permisos según corresponda?».
 *
 * ── SON DOS COSAS, Y LA PANTALLA LO DICE ─────────────────────────────────────────────────────
 *
 * El PERMISO dice qué pantalla puede abrir y vive en Seguridad; el ALCANCE dice qué datos ve y
 * vive acá. Asignar el alcance y olvidar el permiso es el error que se comete la primera vez: la
 * persona queda mirando un 403 y quien se lo asignó cree que lo dejó listo. Por eso cada mitad
 * avisa si le falta su permiso, en vez de dejar que se descubra por teléfono.
 *
 * ── MARCAR UN ÁREA TAMBIÉN DA PODER DE APROBAR EN CxP ────────────────────────────────────────
 *
 * Las áreas se guardan en la MISMA tabla con la que CxP decide quién valida las facturas del
 * departamento. No se duplica el dato a propósito —el responsable del área es el responsable del
 * área, y dos listas separadas se desincronizan—, pero la consecuencia hay que decirla: esto no
 * es solo lectura. Esconderlo sería repartir poder de aprobación de pagos sin que quien lo
 * reparte se entere.
 *
 * ── SOLO SE OFRECEN PARTIDAS DE INGRESO ──────────────────────────────────────────────────────
 *
 * «Mi ingreso» mide únicamente los conceptos de naturaleza INGRESO. Ofrecer una de gasto dejaría
 * a la persona con su partida nombrada arriba y la tabla vacía debajo, sin explicación. El
 * servidor también las rechaza; acá ni siquiera se ofrecen.
 */
import { useEffect, useMemo, useState } from "react";

import {
  Badge,
  Button,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  ErrorState,
  LoadingState,
  useToast,
} from "@/components/ui";
import { BuscadorMultiple } from "@/components/ui";
import type { AreaAsignada } from "@/api/bancos";
import {
  useAlcanceDeUsuario,
  useClasificaciones,
  useConceptos,
  useDepartamentosDeBancos,
  useGuardarAlcanceDeUsuario,
} from "@/features/bancos/hooks";
import { mensajeError } from "@/lib/apiError";

interface Props {
  usuarioId: string;
  onCerrar: () => void;
}

export function AlcanceDelUsuarioPanel({ usuarioId, onCerrar }: Props) {
  const toast = useToast();
  const q = useAlcanceDeUsuario(usuarioId);
  const guardar = useGuardarAlcanceDeUsuario(usuarioId);
  const deptosQ = useDepartamentosDeBancos();
  const clasifQ = useClasificaciones();
  const conceptosQ = useConceptos();

  // Lo elegido vive en el componente para poder mandar la lista completa de una vez. Se siembra
  // con lo que trae el servidor y se vuelve a sembrar cuando la respuesta cambia.
  const [areas, setAreas] = useState<string[]>([]);
  const [partidas, setPartidas] = useState<string[]>([]);
  useEffect(() => {
    if (!q.data) return;
    setAreas(q.data.areas.map((a) => a.departamento_id));
    setPartidas(q.data.partidas.map((p) => p.clasificacion_id));
  }, [q.data]);

  const opcAreas = useMemo(
    () => (deptosQ.data ?? []).map((d) => ({ value: d.id, label: d.nombre })),
    [deptosQ.data],
  );

  // SOLO las partidas cuyo concepto está declarado como INGRESO.
  const opcPartidas = useMemo(() => {
    const deIngreso = new Set(
      (conceptosQ.data ?? []).filter((c) => c.naturaleza === "INGRESO").map((c) => c.id),
    );
    return (clasifQ.data ?? [])
      .filter((c) => deIngreso.has(c.concepto_id))
      .map((c) => ({ value: c.id, label: `${c.concepto} › ${c.nombre}` }));
  }, [clasifQ.data, conceptosQ.data]);

  // El rol en el área se conserva: quien era SUPLENTE sigue siéndolo al guardar otra cosa.
  function rolDe(departamentoId: string): "TITULAR" | "SUPLENTE" {
    const previo = q.data?.areas.find((a) => a.departamento_id === departamentoId);
    return previo?.rol === "SUPLENTE" ? "SUPLENTE" : "TITULAR";
  }

  function guardarAreas() {
    const payload: AreaAsignada[] = areas.map((id) => ({ departamento_id: id, rol: rolDe(id) }));
    guardar.areas.mutate(payload, {
      onSuccess: () => toast.success("Áreas guardadas."),
      onError: (e) => toast.error(mensajeError(e)),
    });
  }

  function guardarPartidas() {
    guardar.partidas.mutate(partidas, {
      onSuccess: () => toast.success("Partidas guardadas."),
      onError: (e) => toast.error(mensajeError(e)),
    });
  }

  if (q.isPending) return <LoadingState label="Cargando el alcance" />;
  if (q.isError) {
    return <ErrorState message={mensajeError(q.error)} onRetry={() => void q.refetch()} />;
  }
  const a = q.data;

  return (
    <Card>
      <CardHeader className="flex flex-row items-start justify-between gap-3">
        <div>
          <CardTitle>Qué ve {a.nombre}</CardTitle>
          <p className="mt-1 text-sm text-content-muted">
            El permiso define qué pantalla abre; esto define qué datos alcanza. Se configura por
            persona, no por rol.
          </p>
        </div>
        <Button size="sm" variant="ghost" onClick={onCerrar}>
          Cerrar
        </Button>
      </CardHeader>

      <CardContent className="flex flex-col gap-6">
        {/* ── GASTO ───────────────────────────────────────────────────────── */}
        <section className="flex flex-col gap-3">
          <div className="flex flex-wrap items-center gap-2">
            <h3 className="font-medium">Áreas a cargo</h3>
            <Badge tone="neutral">Mi presupuesto · gasto</Badge>
            {!a.puede_ver_presupuesto && areas.length > 0 && (
              <Badge tone="pendiente">le falta el permiso</Badge>
            )}
          </div>

          {!a.puede_ver_presupuesto && areas.length > 0 && (
            <p className="rounded-lg border border-pendiente/40 bg-pendiente/5 px-3 py-2 text-sm">
              Tiene áreas asignadas pero su rol no incluye <b>bancos.ver_mi_presupuesto</b>, así
              que no puede abrir «Mi presupuesto». Se marca en Configuración › Seguridad.
            </p>
          )}

          <BuscadorMultiple
            label="Áreas"
            placeholder="Buscar área..."
            opciones={opcAreas}
            seleccion={areas}
            onChange={setAreas}
          />

          {/* La consecuencia que no se puede esconder. */}
          <p className="rounded-lg border border-border bg-surface-muted px-3 py-2 text-sm">
            <b>Ojo:</b> ser responsable de un área también la habilita para <b>validar las
            facturas</b> de esa área en Cuentas por Pagar. Es la misma lista que CxP › Departamentos.
          </p>

          <div>
            <Button size="sm" onClick={guardarAreas} loading={guardar.areas.isPending}>
              Guardar áreas
            </Button>
          </div>
        </section>

        <hr className="border-border" />

        {/* ── INGRESO ─────────────────────────────────────────────────────── */}
        <section className="flex flex-col gap-3">
          <div className="flex flex-wrap items-center gap-2">
            <h3 className="font-medium">Partidas de cobro</h3>
            <Badge tone="neutral">Mi ingreso</Badge>
            {!a.puede_ver_ingreso && partidas.length > 0 && (
              <Badge tone="pendiente">le falta el permiso</Badge>
            )}
          </div>

          {!a.puede_ver_ingreso && partidas.length > 0 && (
            <p className="rounded-lg border border-pendiente/40 bg-pendiente/5 px-3 py-2 text-sm">
              Tiene partidas asignadas pero su rol no incluye <b>bancos.ver_mi_ingreso</b>, así que
              no puede abrir «Mi ingreso». Se marca en Configuración › Seguridad.
            </p>
          )}

          <BuscadorMultiple
            label="Partidas de ingreso"
            placeholder="Buscar partida..."
            opciones={opcPartidas}
            seleccion={partidas}
            onChange={setPartidas}
          />

          {opcPartidas.length === 0 && (
            <p className="rounded-lg border border-border bg-surface-muted px-3 py-2 text-sm">
              Todavía no hay ningún concepto declarado como <b>ingreso</b> en el catálogo, así que
              no hay partidas que asignar. Se declara en Bancos › Catálogo.
            </p>
          )}

          <div>
            <Button size="sm" onClick={guardarPartidas} loading={guardar.partidas.isPending}>
              Guardar partidas
            </Button>
          </div>
        </section>
      </CardContent>
    </Card>
  );
}
