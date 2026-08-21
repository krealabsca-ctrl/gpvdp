/**
 * Pantalla 6 — Catálogo (/catalogo).
 * Pestañas Cuentas / Conceptos / Clasificaciones. Conceptos y Clasificaciones
 * permiten crear (POST). Crear cuentas queda para más adelante.
 */

import { useState, type FormEvent } from "react";
import {
  Badge,
  Button,
  Card,
  CardContent,
  EmptyState,
  ErrorState,
  Input,
  LoadingState,
  PageHeader,
  Select,
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
import { mensajeError } from "@/lib/apiError";
import { sinTildes } from "@/lib/format";
import type { Moneda } from "@/lib/format";
import { ConfirmDialog } from "@/components/ui";
import { etiquetaNaturaleza } from "@/api/bancos";
import type {
  BancoItem,
  CambioDeCuenta,
  ClasificacionCatalogo,
  ConceptoCatalogo,
  CuentaBancaria,
  NaturalezaConcepto,
  ResumenFusion,
} from "@/api/bancos";
import { DiccionarioPanel } from "@/features/bancos/components/DiccionarioPanel";
import {
  useActualizarCuenta,
  useBancosCatalogo,
  useCambiarActivoBanco,
  useCambiarActivoCuenta,
  useCambiarNaturaleza,
  useCambiarVisibilidadCxP,
  useClasificaciones,
  useConceptos,
  useCrearBanco,
  useCrearClasificacion,
  useCrearConcepto,
  useCrearCuenta,
  useCuentas,
  useEliminarBanco,
  useEliminarClasificacion,
  useEliminarConcepto,
  useEliminarCuenta,
  useFusionarClasificacion,
  useFusionarConcepto,
  useRenombrarBanco,
  useReasignarConceptoClasificacion,
  useRenombrarClasificacion,
  useRenombrarConcepto,
  useUsoDeCuenta,
} from "@/features/bancos/hooks";

type Tab = "cuentas" | "conceptos" | "clasificaciones" | "diccionario";

// Las REGLAS del motor se gestionan en la Bandeja de clasificación (/clasificar),
// junto al aprendizaje; aquí queda solo el catálogo estructural… salvo el Diccionario,
// que importa catálogo Y reglas de una vez (el archivo trae las palabras clave).
const TABS: { id: Tab; label: string }[] = [
  { id: "cuentas", label: "Cuentas" },
  { id: "conceptos", label: "Conceptos" },
  { id: "clasificaciones", label: "Clasificaciones" },
  { id: "diccionario", label: "Importar / Exportar" },
];

export function CatalogoPage() {
  const [tab, setTab] = useState<Tab>("cuentas");

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title="Catálogo"
        description="Cuentas bancarias, conceptos y clasificaciones de la empresa activa."
      />

      <div role="tablist" aria-label="Secciones del catálogo" className="flex gap-1 border-b border-border">
        {TABS.map((t) => (
          <button
            key={t.id}
            role="tab"
            aria-selected={tab === t.id}
            id={`tab-${t.id}`}
            aria-controls={`panel-${t.id}`}
            onClick={() => setTab(t.id)}
            className={cn(
              "-mb-px border-b-2 px-4 py-2 text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent",
              tab === t.id
                ? "border-accent text-accent"
                : "border-transparent text-content-muted hover:text-content",
            )}
          >
            {t.label}
          </button>
        ))}
      </div>

      <div role="tabpanel" id={`panel-${tab}`} aria-labelledby={`tab-${tab}`}>
        {tab === "cuentas" && <CuentasTab />}
        {tab === "conceptos" && <ConceptosTab />}
        {tab === "clasificaciones" && <ClasificacionesTab />}
        {tab === "diccionario" && <DiccionarioPanel />}
      </div>
    </div>
  );
}

function CuentasTab() {
  return (
    <div className="flex flex-col gap-6">
      <BancosSection />
      <CuentasSection />
    </div>
  );
}

function BancosSection() {
  const toast = useToast();
  // El catálogo SÍ pide los inactivos: si no, un banco desactivado por error no se podría
  // volver a encender desde ninguna pantalla.
  const q = useBancosCatalogo(true);
  const crear = useCrearBanco();
  const renombrar = useRenombrarBanco();
  const eliminar = useEliminarBanco();
  const activar = useCambiarActivoBanco();
  const [nombre, setNombre] = useState("");
  const [editId, setEditId] = useState<string | null>(null);
  const [editNombre, setEditNombre] = useState("");
  const [porEliminar, setPorEliminar] = useState<BancoItem | null>(null);

  const bancos = q.data ?? [];

  function onCrear(e: FormEvent) {
    e.preventDefault();
    const n = nombre.trim();
    if (!n) return;
    crear.mutate(n, {
      onSuccess: () => {
        toast.success("Banco creado.");
        setNombre("");
      },
      onError: (err) => toast.error(mensajeError(err)),
    });
  }

  function guardar(id: string) {
    const n = editNombre.trim();
    if (!n) return;
    renombrar.mutate(
      { id, nombre: n },
      {
        onSuccess: () => {
          toast.success("Banco renombrado.");
          setEditId(null);
        },
        onError: (err) => toast.error(mensajeError(err)),
      },
    );
  }

  return (
    <Card>
      <CardContent className="flex flex-col gap-4 py-4">
        <div>
          <h3 className="text-sm font-semibold text-content">Bancos</h3>
          <p className="text-xs text-content-muted">
            Registrá los bancos de la empresa. Las cuentas se crean bajo un banco.
          </p>
        </div>
        <form onSubmit={onCrear} className="flex flex-wrap items-end gap-3">
          <div className="min-w-[220px] flex-1">
            <Input
              label="Nuevo banco"
              value={nombre}
              onChange={(e) => setNombre(e.target.value)}
              placeholder="Ej. BAC San José"
            />
          </div>
          <Button type="submit" loading={crear.isPending} disabled={!nombre.trim()}>
            Crear banco
          </Button>
        </form>

        {q.isPending ? (
          <LoadingState label="Cargando bancos" />
        ) : q.isError ? (
          <ErrorState message={mensajeError(q.error)} onRetry={() => q.refetch()} />
        ) : bancos.length === 0 ? (
          <EmptyState message="No hay bancos registrados." />
        ) : (
          <TableContainer>
            <Table>
              <THead>
                <TR>
                  <TH>Banco</TH>
                  <TH className="text-right">Acción</TH>
                </TR>
              </THead>
              <TBody>
                {bancos.map((b) => (
                  <TR key={b.id} className={cn(!b.activo && "opacity-60")}>
                    <TD className="font-medium">
                      {editId === b.id ? (
                        <Input
                          value={editNombre}
                          onChange={(e) => setEditNombre(e.target.value)}
                          className="max-w-xs"
                          aria-label="Nuevo nombre del banco"
                        />
                      ) : (
                        <>
                          {b.nombre}
                          {!b.activo && (
                            <Badge tone="neutral" className="ml-2">
                              desactivado
                            </Badge>
                          )}
                        </>
                      )}
                    </TD>
                    <TD className="text-right">
                      {editId === b.id ? (
                        <div className="flex justify-end gap-2">
                          <Button
                            size="sm"
                            onClick={() => guardar(b.id)}
                            loading={renombrar.isPending}
                            disabled={!editNombre.trim()}
                          >
                            Guardar
                          </Button>
                          <Button size="sm" variant="ghost" onClick={() => setEditId(null)}>
                            Cancelar
                          </Button>
                        </div>
                      ) : (
                        <div className="flex justify-end gap-2">
                          <Button
                            size="sm"
                            variant="secondary"
                            onClick={() => {
                              setEditId(b.id);
                              setEditNombre(b.nombre);
                            }}
                          >
                            Renombrar
                          </Button>
                          <Button
                            size="sm"
                            variant="ghost"
                            loading={activar.isPending}
                            onClick={() =>
                              activar.mutate(
                                { id: b.id, activo: !b.activo },
                                {
                                  onSuccess: () =>
                                    toast.success(b.activo ? "Banco desactivado." : "Banco reactivado."),
                                  onError: (err) => toast.error(mensajeError(err)),
                                },
                              )
                            }
                          >
                            {b.activo ? "Desactivar" : "Reactivar"}
                          </Button>
                          <Button size="sm" variant="ghost" onClick={() => setPorEliminar(b)}>
                            Eliminar
                          </Button>
                        </div>
                      )}
                    </TD>
                  </TR>
                ))}
              </TBody>
            </Table>
          </TableContainer>
        )}

        {/* Eliminar es FÍSICO y solo si el banco no tiene cuentas; si tiene, el servidor
            responde con el detalle y la salida es desactivarlo. */}
        {porEliminar && (
          <ConfirmDialog
            titulo={`¿Eliminar «${porEliminar.nombre}»?`}
            descripcion="Solo se puede eliminar un banco que no tenga cuentas. Si tiene, desactivalo en su lugar."
            impacto={["El banco desaparece del catálogo", "No se puede deshacer"]}
            textoConfirmar="Eliminar banco"
            tono="peligro"
            pendiente={eliminar.isPending}
            onConfirmar={() =>
              eliminar.mutate(porEliminar.id, {
                onSuccess: () => {
                  toast.success("Banco eliminado.");
                  setPorEliminar(null);
                },
                onError: (err) => toast.error(mensajeError(err)),
              })
            }
            onCancelar={() => setPorEliminar(null)}
          />
        )}
      </CardContent>
    </Card>
  );
}

function CuentasSection() {
  const toast = useToast();
  // Con inactivas: el catálogo es el único lugar desde donde se reactivan.
  const q = useCuentas(true);
  const bancosQ = useBancosCatalogo(true);
  const crear = useCrearCuenta();
  const actualizar = useActualizarCuenta();
  const activar = useCambiarActivoCuenta();
  const [bancoId, setBancoId] = useState("");
  const [alias, setAlias] = useState("");
  const [iban, setIban] = useState("");
  const [moneda, setMoneda] = useState<Moneda>("CRC");
  const [editId, setEditId] = useState<string | null>(null);
  const [editAlias, setEditAlias] = useState("");
  const [editIban, setEditIban] = useState("");
  const [editMoneda, setEditMoneda] = useState<Moneda>("CRC");
  const [editBanco, setEditBanco] = useState("");
  const [porEliminar, setPorEliminar] = useState<CuentaBancaria | null>(null);
  // Qué cuelga de la cuenta que se está editando: decide si la moneda y el IBAN se pueden
  // tocar, y se pide ANTES de que el usuario intente guardar.
  const usoQ = useUsoDeCuenta(editId ?? "");
  const conMovimientos = (usoQ.data?.movimientos ?? 0) > 0;

  const cuentas = q.data ?? [];
  // Para crear o mover una cuenta solo se ofrecen los bancos activos.
  const bancoOptions = (bancosQ.data ?? []).filter((b) => b.activo).map((b) => ({ value: b.id, label: b.nombre }));

  function onCrear(e: FormEvent) {
    e.preventDefault();
    if (!bancoId) {
      toast.error("Elegí un banco.");
      return;
    }
    if (!alias.trim()) {
      toast.error("Ingresá un alias para la cuenta.");
      return;
    }
    crear.mutate(
      { banco_id: bancoId, alias: alias.trim(), iban: iban.trim() || undefined, moneda },
      {
        onSuccess: () => {
          toast.success("Cuenta creada.");
          setAlias("");
          setIban("");
        },
        onError: (err) => toast.error(mensajeError(err)),
      },
    );
  }

  function abrirEdicion(c: CuentaBancaria) {
    setEditId(c.id);
    setEditAlias(c.alias);
    setEditIban(c.iban);
    setEditMoneda(c.moneda);
    setEditBanco("");
  }

  /**
   * Solo se manda lo que cambió. La moneda y el IBAN se mandan únicamente si la cuenta no
   * tiene movimientos: el servidor lo vuelve a verificar, pero la pantalla no debería ni
   * ofrecerlo (cambiarlos reinterpretaría montos que ya están en el cuadre).
   */
  function guardar(c: CuentaBancaria) {
    const cambio: CambioDeCuenta = {};
    const a = editAlias.trim();
    if (a && a !== c.alias) cambio.alias = a;
    if (editBanco) cambio.banco_id = editBanco;
    if (!conMovimientos) {
      if (editIban.trim() !== c.iban) cambio.iban = editIban.trim();
      if (editMoneda !== c.moneda) cambio.moneda = editMoneda;
    }
    if (Object.keys(cambio).length === 0) {
      setEditId(null);
      return;
    }
    actualizar.mutate(
      { id: c.id, cambio },
      {
        onSuccess: () => {
          toast.success("Cuenta actualizada.");
          setEditId(null);
        },
        onError: (err) => toast.error(mensajeError(err)),
      },
    );
  }

  return (
    <Card>
      <CardContent className="flex flex-col gap-4 py-4">
        <div>
          <h3 className="text-sm font-semibold text-content">Cuentas</h3>
          <p className="text-xs text-content-muted">
            El importador reconoce el formato del banco automáticamente; el IBAN se valida contra el
            archivo al importar.
          </p>
        </div>
        <form onSubmit={onCrear} className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
          <Select
            label="Banco"
            placeholder={bancosQ.isPending ? "Cargando…" : "Seleccioná…"}
            value={bancoId}
            onChange={(e) => setBancoId(e.target.value)}
            options={bancoOptions}
          />
          <Input
            label="Alias"
            value={alias}
            onChange={(e) => setAlias(e.target.value)}
            placeholder="Ej. BAC Colones"
          />
          <Input
            label="IBAN (opcional)"
            value={iban}
            onChange={(e) => setIban(e.target.value)}
            placeholder="CR00 0000 0000 0000 0000 00"
            className="font-mono"
          />
          <div className="flex items-end gap-2">
            <div className="flex-1">
              <Select
                label="Moneda"
                value={moneda}
                onChange={(e) => setMoneda(e.target.value as Moneda)}
                options={[
                  { value: "CRC", label: "CRC" },
                  { value: "USD", label: "USD" },
                ]}
              />
            </div>
            <Button type="submit" loading={crear.isPending} disabled={!bancoId || !alias.trim()}>
              Crear
            </Button>
          </div>
        </form>
        {bancoOptions.length === 0 && !bancosQ.isPending && (
          <p className="text-sm text-content-muted">Primero creá un banco arriba.</p>
        )}

        {q.isPending ? (
          <LoadingState label="Cargando cuentas" />
        ) : q.isError ? (
          <ErrorState message={mensajeError(q.error)} onRetry={() => q.refetch()} />
        ) : cuentas.length === 0 ? (
          <EmptyState message="No hay cuentas registradas." />
        ) : (
          <TableContainer>
            <Table>
              <THead>
                <TR>
                  <TH>Alias</TH>
                  <TH>Banco</TH>
                  <TH>IBAN</TH>
                  <TH>Moneda</TH>
                  <TH className="text-right">Acción</TH>
                </TR>
              </THead>
              <TBody>
                {cuentas.map((c) => (
                  <TR key={c.id} className={cn(!c.activo && "opacity-60")}>
                    <TD className="font-medium">
                      {editId === c.id ? (
                        <Input
                          value={editAlias}
                          onChange={(e) => setEditAlias(e.target.value)}
                          className="max-w-[12rem]"
                          aria-label="Nuevo alias de la cuenta"
                        />
                      ) : (
                        <>
                          {c.alias}
                          {!c.activo && (
                            <Badge tone="neutral" className="ml-2">
                              desactivada
                            </Badge>
                          )}
                        </>
                      )}
                    </TD>
                    <TD>
                      {editId === c.id ? (
                        <Select
                          value={editBanco}
                          onChange={(e) => setEditBanco(e.target.value)}
                          options={[{ value: "", label: c.banco }, ...bancoOptions]}
                          className="min-w-[10rem]"
                          aria-label="Mover la cuenta a otro banco"
                        />
                      ) : (
                        c.banco
                      )}
                    </TD>
                    <TD className="font-mono text-xs">
                      {editId === c.id && !conMovimientos ? (
                        <Input
                          value={editIban}
                          onChange={(e) => setEditIban(e.target.value)}
                          className="max-w-[13rem] font-mono"
                          aria-label="Nuevo IBAN"
                        />
                      ) : (
                        c.iban || "—"
                      )}
                    </TD>
                    <TD>
                      {editId === c.id && !conMovimientos ? (
                        <Select
                          value={editMoneda}
                          onChange={(e) => setEditMoneda(e.target.value as Moneda)}
                          options={[
                            { value: "CRC", label: "CRC" },
                            { value: "USD", label: "USD" },
                          ]}
                          className="min-w-[5.5rem]"
                          aria-label="Nueva moneda"
                        />
                      ) : (
                        <Badge tone="accent">{c.moneda}</Badge>
                      )}
                    </TD>
                    <TD className="text-right">
                      {editId === c.id ? (
                        <div className="flex flex-col items-end gap-1">
                          <div className="flex justify-end gap-2">
                            <Button
                              size="sm"
                              onClick={() => guardar(c)}
                              loading={actualizar.isPending}
                              disabled={!editAlias.trim()}
                            >
                              Guardar
                            </Button>
                            <Button size="sm" variant="ghost" onClick={() => setEditId(null)}>
                              Cancelar
                            </Button>
                          </div>
                          {/* Por qué la moneda y el IBAN están bloqueados: no es un capricho
                              de la pantalla, es que ya hay montos importados con esa moneda. */}
                          {conMovimientos && (
                            <span className="text-[11px] text-content-muted">
                              {usoQ.data?.movimientos.toLocaleString("es-CR")} movimientos: la moneda y el IBAN ya
                              no se pueden cambiar
                            </span>
                          )}
                        </div>
                      ) : (
                        <div className="flex justify-end gap-2">
                          <Button size="sm" variant="secondary" onClick={() => abrirEdicion(c)}>
                            Editar
                          </Button>
                          <Button
                            size="sm"
                            variant="ghost"
                            loading={activar.isPending}
                            onClick={() =>
                              activar.mutate(
                                { id: c.id, activo: !c.activo },
                                {
                                  onSuccess: () =>
                                    toast.success(c.activo ? "Cuenta desactivada." : "Cuenta reactivada."),
                                  onError: (err) => toast.error(mensajeError(err)),
                                },
                              )
                            }
                          >
                            {c.activo ? "Desactivar" : "Reactivar"}
                          </Button>
                          <Button size="sm" variant="ghost" onClick={() => setPorEliminar(c)}>
                            Eliminar
                          </Button>
                        </div>
                      )}
                    </TD>
                  </TR>
                ))}
              </TBody>
            </Table>
          </TableContainer>
        )}

        {porEliminar && <DialogoEliminarCuenta cuenta={porEliminar} onCerrar={() => setPorEliminar(null)} />}
      </CardContent>
    </Card>
  );
}

/**
 * Antes de ofrecer el borrado se consulta qué cuelga de la cuenta: así el diálogo dice si
 * se va a poder o no, en vez de dejar al usuario chocar con un error después de confirmar.
 */
function DialogoEliminarCuenta({ cuenta, onCerrar }: { cuenta: CuentaBancaria; onCerrar: () => void }) {
  const toast = useToast();
  const eliminar = useEliminarCuenta();
  const activar = useCambiarActivoCuenta();
  const usoQ = useUsoDeCuenta(cuenta.id);
  const uso = usoQ.data;
  const libre =
    uso !== undefined &&
    uso.movimientos === 0 &&
    uso.importaciones === 0 &&
    uso.saldos === 0 &&
    uso.actas === 0 &&
    uso.partidas === 0;

  const detalle = uso
    ? [
        uso.movimientos > 0 && `${uso.movimientos.toLocaleString("es-CR")} movimientos`,
        uso.importaciones > 0 && `${uso.importaciones} importaciones`,
        uso.saldos > 0 && `${uso.saldos} saldos diarios`,
        uso.actas > 0 && `${uso.actas} actas de conciliación`,
        uso.partidas > 0 && `${uso.partidas} partidas`,
      ]
        .filter(Boolean)
        .join(" · ")
    : "";

  return (
    <ConfirmDialog
      titulo={libre ? `¿Eliminar «${cuenta.alias}»?` : `«${cuenta.alias}» no se puede eliminar`}
      descripcion={
        usoQ.isPending
          ? "Revisando qué tiene la cuenta…"
          : libre
            ? "La cuenta no tiene movimientos ni historia: se puede borrar del todo."
            : `Tiene ${detalle}. Se puede desactivar: deja de aparecer en el importador y en los filtros, pero su historia y su cuadre se conservan.`
      }
      impacto={libre ? ["No se puede deshacer"] : ["Desactivar no borra nada", "Se puede reactivar después"]}
      textoConfirmar={libre ? "Eliminar cuenta" : cuenta.activo ? "Desactivar cuenta" : "Reactivar cuenta"}
      tono={libre ? "peligro" : "accent"}
      pendiente={eliminar.isPending || activar.isPending || usoQ.isPending}
      onConfirmar={() => {
        const opciones = {
          onSuccess: () => {
            toast.success(libre ? "Cuenta eliminada." : cuenta.activo ? "Cuenta desactivada." : "Cuenta reactivada.");
            onCerrar();
          },
          onError: (err: unknown) => toast.error(mensajeError(err)),
        };
        if (libre) eliminar.mutate(cuenta.id, opciones);
        else activar.mutate({ id: cuenta.id, activo: !cuenta.activo }, opciones);
      }}
      onCancelar={onCerrar}
    />
  );
}

function ConceptosTab() {
  const toast = useToast();
  const q = useConceptos();
  const crear = useCrearConcepto();
  const renombrar = useRenombrarConcepto();
  const eliminar = useEliminarConcepto();
  const visibilidad = useCambiarVisibilidadCxP();
  const naturalezaM = useCambiarNaturaleza();
  const fusionar = useFusionarConcepto();
  const [nombre, setNombre] = useState("");
  const [nuevoVisibleCxP, setNuevoVisibleCxP] = useState(false);
  const [editId, setEditId] = useState<string | null>(null);
  const [editNombre, setEditNombre] = useState("");
  const [confirmandoId, setConfirmandoId] = useState<string | null>(null);
  const [fusionando, setFusionando] = useState<ConceptoCatalogo | null>(null);

  function onSubmit(e: FormEvent) {
    e.preventDefault();
    const n = nombre.trim();
    if (!n) return;
    crear.mutate({ nombre: n, visible_cxp: nuevoVisibleCxP }, { onSuccess: () => setNombre("") });
  }

  /**
   * Declara qué es el concepto para el EBITDA. Cambia el número de TODOS los períodos, así que el
   * aviso lo dice: no es una etiqueta cosmética.
   */
  function cambiarNaturalezaDe(c: ConceptoCatalogo, naturaleza: NaturalezaConcepto) {
    // La guarda solo aplica a lo YA declarado. Si nadie declaró nada, elegir el mismo valor que
    // muestra el selector SÍ es un cambio —pasa de «falta decidir» a «decidido»— y antes no se podía:
    // el clic sobre la opción ya seleccionada no hacía nada y el aviso del tablero no bajaba nunca.
    if (naturaleza === c.naturaleza && c.naturaleza_declarada) return;
    naturalezaM.mutate(
      { id: c.id, naturaleza },
      {
        onSuccess: () =>
          toast.success(
            naturaleza === "NEUTRO"
              ? `«${c.nombre}» queda declarado FUERA del EBITDA. Ya no cuenta como pendiente.`
              : `«${c.nombre}» cuenta como ${naturaleza === "INGRESO" ? "ingreso" : "gasto"} en el EBITDA de todos los períodos.`,
          ),
        onError: (err) => toast.error(mensajeError(err)),
      },
    );
  }

  function toggleVisibilidad(id: string, visible: boolean, nombreConcepto: string) {
    visibilidad.mutate(
      { id, visible },
      {
        onSuccess: () =>
          toast.success(
            visible
              ? `«${nombreConcepto}» ahora es visible en CxP.`
              : `«${nombreConcepto}» quedó oculto para CxP (contabilidad ya no lo ve).`,
          ),
        onError: (err) => toast.error(mensajeError(err)),
      },
    );
  }

  function guardar(id: string) {
    const n = editNombre.trim();
    if (!n) return;
    renombrar.mutate(
      { id, nombre: n },
      {
        onSuccess: () => {
          toast.success("Concepto renombrado.");
          setEditId(null);
        },
        onError: (err) => toast.error(mensajeError(err)),
      },
    );
  }

  function borrar(id: string) {
    eliminar.mutate(id, {
      onSuccess: () => {
        toast.success("Concepto eliminado.");
        setConfirmandoId(null);
      },
      onError: (err) => {
        toast.error(mensajeError(err));
        setConfirmandoId(null);
      },
    });
  }

  const conceptos = q.data ?? [];

  return (
    <div className="flex flex-col gap-4">
      <Card>
        <CardContent className="py-4">
          <form onSubmit={onSubmit} className="flex flex-wrap items-end gap-3">
            <div className="min-w-[220px] flex-1">
              <Input
                label="Nuevo concepto"
                value={nombre}
                onChange={(e) => setNombre(e.target.value)}
                placeholder="Ej. Ingresos"
              />
            </div>
            <label className="flex items-center gap-2 pb-2.5 text-sm text-content-muted">
              <input
                type="checkbox"
                checked={nuevoVisibleCxP}
                onChange={(e) => setNuevoVisibleCxP(e.target.checked)}
                className="h-4 w-4 rounded border-border accent-accent"
              />
              Visible en CxP
            </label>
            <Button type="submit" loading={crear.isPending} disabled={!nombre.trim()}>
              Crear concepto
            </Button>
          </form>
          {crear.isError && <p className="mt-2 text-sm text-negativo">{mensajeError(crear.error)}</p>}
        </CardContent>
      </Card>

      {q.isPending ? (
        <LoadingState label="Cargando conceptos" />
      ) : q.isError ? (
        <ErrorState message={mensajeError(q.error)} onRetry={() => q.refetch()} />
      ) : conceptos.length === 0 ? (
        <EmptyState message="No hay conceptos registrados." />
      ) : (
        <TableContainer>
          <Table>
            <THead>
              <TR>
                <TH>Nombre</TH>
                {/* La naturaleza es lo que decide el EBITDA: el dashboard suma como ingreso SOLO
                    los conceptos declarados INGRESO y como gasto solo los GASTO. */}
                <TH>En el EBITDA</TH>
                <TH className="text-center">Visible en CxP</TH>
                <TH className="text-right">Acción</TH>
              </TR>
            </THead>
            <TBody>
              {conceptos.map((c) => (
                <TR key={c.id}>
                  <TD className="font-medium">
                    {editId === c.id ? (
                      <Input
                        value={editNombre}
                        onChange={(e) => setEditNombre(e.target.value)}
                        className="max-w-xs"
                        aria-label="Nuevo nombre del concepto"
                      />
                    ) : (
                      c.nombre
                    )}
                  </TD>
                  <TD>
                    <Select
                      aria-label={`Naturaleza de ${c.nombre} en el EBITDA`}
                      value={c.naturaleza_declarada ? c.naturaleza : ""}
                      onChange={(e) => cambiarNaturalezaDe(c, e.target.value as NaturalezaConcepto)}
                      disabled={naturalezaM.isPending}
                      options={[
                        // La opción vacía existe solo mientras nadie declaró: mostrar «— No entra»
                        // seleccionado le informaba al usuario una decisión que nadie había tomado.
                        ...(c.naturaleza_declarada ? [] : [{ value: "", label: "· sin declarar" }]),
                        { value: "INGRESO", label: "↑ Ingreso" },
                        { value: "GASTO", label: "↓ Gasto" },
                        { value: "NEUTRO", label: "— No entra" },
                      ]}
                      className="min-w-32"
                    />
                    <span className="mt-0.5 block text-[11px] text-content-muted">
                      {c.naturaleza_declarada
                        ? etiquetaNaturaleza(c.naturaleza)
                        : "Falta decidirlo: mientras tanto queda fuera del EBITDA"}
                    </span>
                  </TD>
                  <TD className="text-center">
                    <button
                      type="button"
                      role="switch"
                      aria-checked={c.visible_cxp}
                      onClick={() => toggleVisibilidad(c.id, !c.visible_cxp, c.nombre)}
                      disabled={visibilidad.isPending}
                      className={cn(
                        "relative inline-flex h-5 w-9 items-center rounded-full transition-colors",
                        c.visible_cxp ? "bg-accent" : "border border-border bg-surface-muted",
                      )}
                      title={
                        c.visible_cxp
                          ? "Contabilidad LO VE en el clasificador de gastos de CxP — clic para ocultarlo"
                          : "Oculto para CxP (solo catálogo bancario) — clic para mostrarlo"
                      }
                    >
                      <span
                        className="inline-block h-3.5 w-3.5 rounded-full bg-surface-raised shadow transition-transform"
                        style={{ transform: c.visible_cxp ? "translateX(18px)" : "translateX(2px)" }}
                      />
                    </button>
                  </TD>
                  <TD className="text-right">
                    {editId === c.id ? (
                      <div className="flex justify-end gap-2">
                        <Button size="sm" onClick={() => guardar(c.id)} loading={renombrar.isPending} disabled={!editNombre.trim()}>
                          Guardar
                        </Button>
                        <Button size="sm" variant="ghost" onClick={() => setEditId(null)}>
                          Cancelar
                        </Button>
                      </div>
                    ) : confirmandoId === c.id ? (
                      <div className="flex items-center justify-end gap-2">
                        <span className="text-xs text-content-muted">¿Eliminar «{c.nombre}»?</span>
                        <Button size="sm" variant="secondary" className="text-negativo" onClick={() => borrar(c.id)} loading={eliminar.isPending}>
                          Sí
                        </Button>
                        <Button size="sm" variant="ghost" onClick={() => setConfirmandoId(null)}>
                          No
                        </Button>
                      </div>
                    ) : (
                      <div className="flex justify-end gap-2">
                        <Button
                          size="sm"
                          variant="secondary"
                          onClick={() => {
                            setEditId(c.id);
                            setEditNombre(c.nombre);
                          }}
                        >
                          Renombrar
                        </Button>
                        <Button size="sm" variant="ghost" onClick={() => setFusionando(c)}>
                          Fusionar…
                        </Button>
                        <Button size="sm" variant="ghost" className="text-negativo" onClick={() => setConfirmandoId(c.id)}>
                          Eliminar
                        </Button>
                      </div>
                    )}
                  </TD>
                </TR>
              ))}
            </TBody>
          </Table>
        </TableContainer>
      )}

      {fusionando && (
        <PanelFusion
          titulo={`Fusionar «${fusionando.nombre}» con otro concepto`}
          explicacion="Todos sus movimientos, reglas, documentos de CxP, vales de caja chica y gastos de proveedor pasan al concepto destino, con sus clasificaciones. Después el concepto de origen se elimina."
          opciones={conceptos.filter((x) => x.id !== fusionando.id).map((x) => ({ value: x.id, label: x.nombre }))}
          pendiente={fusionar.isPending}
          onCancelar={() => setFusionando(null)}
          onFusionar={(destinoId) =>
            fusionar.mutate(
              { id: fusionando.id, destinoId },
              {
                onSuccess: (res) => {
                  toast.success(resumenFusionTexto(res));
                  setFusionando(null);
                },
                onError: (err) => toast.error(mensajeError(err)),
              },
            )
          }
        />
      )}

      <p className="text-xs text-content-muted">
        <strong>Visible en CxP</strong>: contabilidad solo ve en su clasificador de gastos los conceptos
        encendidos — el resto del catálogo bancario (ingresos, traslados, overnight…) queda privado.
        <strong> Eliminar</strong> solo aplica a conceptos sin uso; si está en uso, usá <strong>Fusionar</strong>:
        le pasa los movimientos a otro concepto y deja el catálogo limpio.
      </p>
    </div>
  );
}

/** Texto del aviso de una fusión: dice exactamente qué se movió. */
function resumenFusionTexto(res: ResumenFusion): string {
  const partes = [
    res.movimientos > 0 && `${res.movimientos.toLocaleString("es-CR")} movimientos`,
    res.reglas > 0 && `${res.reglas} reglas`,
    res.documentos_cxp > 0 && `${res.documentos_cxp} documentos de CxP`,
    res.vales_caja_chica > 0 && `${res.vales_caja_chica} vales`,
    res.gastos_proveedor > 0 && `${res.gastos_proveedor} gastos de proveedor`,
    res.proveedores > 0 && `${res.proveedores} proveedores`,
    res.clasificaciones > 0 && `${res.clasificaciones} clasificaciones`,
  ].filter(Boolean);
  const detalle = partes.length > 0 ? `: ${partes.join(", ")}` : " (no tenía nada asignado)";
  return `«${res.origen}» quedó fusionado en «${res.destino}»${detalle}.`;
}

/**
 * Panel de fusión: elegir el destino y confirmar. Se muestra el destino ANTES de confirmar
 * porque la operación es irreversible y mueve datos de tres módulos.
 */
function PanelFusion({
  titulo,
  explicacion,
  opciones,
  pendiente,
  onFusionar,
  onCancelar,
  avisoExtra,
}: {
  titulo: string;
  explicacion: string;
  opciones: { value: string; label: string }[];
  pendiente: boolean;
  onFusionar: (destinoId: string) => void;
  onCancelar: () => void;
  avisoExtra?: string;
}) {
  const [destino, setDestino] = useState("");
  return (
    <Card>
      <CardContent className="flex flex-col gap-3 py-4">
        <div>
          <h3 className="text-sm font-semibold text-content">{titulo}</h3>
          <p className="text-xs text-content-muted">{explicacion}</p>
        </div>
        {opciones.length === 0 ? (
          <p className="text-sm text-content-muted">No hay otra entrada con la que fusionar.</p>
        ) : (
          <div className="flex flex-wrap items-end gap-3">
            <div className="min-w-[260px] flex-1">
              <Select
                label="Destino"
                placeholder="Elegí a dónde van…"
                value={destino}
                onChange={(e) => setDestino(e.target.value)}
                options={opciones}
              />
            </div>
            <Button onClick={() => onFusionar(destino)} loading={pendiente} disabled={!destino}>
              Fusionar
            </Button>
            <Button variant="ghost" onClick={onCancelar}>
              Cancelar
            </Button>
          </div>
        )}
        {avisoExtra && <p className="text-xs text-pendiente">{avisoExtra}</p>}
        <p className="text-xs text-content-muted">
          La fusión <strong>no se puede deshacer</strong> y queda en la auditoría con el detalle de lo que movió.
        </p>
      </CardContent>
    </Card>
  );
}

function ClasificacionesTab() {
  const toast = useToast();
  const q = useClasificaciones();
  const conceptosQ = useConceptos();
  const crear = useCrearClasificacion();
  const renombrar = useRenombrarClasificacion();
  const reasignar = useReasignarConceptoClasificacion();
  const eliminar = useEliminarClasificacion();
  const fusionarClasif = useFusionarClasificacion();
  const [conceptoId, setConceptoId] = useState("");
  const [nombre, setNombre] = useState("");
  const [editId, setEditId] = useState<string | null>(null);
  const [editNombre, setEditNombre] = useState("");
  const [editConceptoId, setEditConceptoId] = useState("");
  const [confirmandoId, setConfirmandoId] = useState<string | null>(null);
  const [fusionando, setFusionando] = useState<ClasificacionCatalogo | null>(null);
  // Filtros de la tabla: con decenas de clasificaciones, encontrar una a ojo no escala.
  const [filtroConcepto, setFiltroConcepto] = useState("");
  const [busqueda, setBusqueda] = useState("");

  const conceptos = conceptosQ.data ?? [];

  function onSubmit(e: FormEvent) {
    e.preventDefault();
    const n = nombre.trim();
    if (!conceptoId || !n) return;
    crear.mutate({ concepto_id: conceptoId, nombre: n }, { onSuccess: () => setNombre("") });
  }

  async function guardar(c: { id: string; nombre: string; concepto_id: string }) {
    const n = editNombre.trim();
    if (!n) return;
    try {
      // Primero mover de concepto (si cambió), luego renombrar (si cambió).
      if (editConceptoId && editConceptoId !== c.concepto_id) {
        await reasignar.mutateAsync({ id: c.id, conceptoId: editConceptoId });
      }
      if (n !== c.nombre) {
        await renombrar.mutateAsync({ id: c.id, nombre: n });
      }
      toast.success("Clasificación actualizada.");
      setEditId(null);
    } catch (err) {
      toast.error(mensajeError(err));
    }
  }

  function borrar(id: string) {
    eliminar.mutate(id, {
      onSuccess: () => {
        toast.success("Clasificación eliminada.");
        setConfirmandoId(null);
      },
      onError: (err) => {
        toast.error(mensajeError(err));
        setConfirmandoId(null);
      },
    });
  }

  const todas = q.data ?? [];
  // Filtrado insensible a tildes y mayúsculas, igual que el resto del catálogo.
  const clasificaciones = todas.filter((c) => {
    if (filtroConcepto && c.concepto_id !== filtroConcepto) return false;
    if (!busqueda.trim()) return true;
    const t = sinTildes(busqueda);
    return sinTildes(c.nombre).includes(t) || sinTildes(c.concepto).includes(t);
  });

  return (
    <div className="flex flex-col gap-4">
      <Card>
        <CardContent className="py-4">
          <form onSubmit={onSubmit} className="flex flex-wrap items-end gap-3">
            <div className="min-w-[200px] flex-1">
              <Select
                label="Concepto"
                placeholder="Seleccioná…"
                value={conceptoId}
                onChange={(e) => setConceptoId(e.target.value)}
                options={conceptos.map((c) => ({ value: c.id, label: c.nombre }))}
              />
            </div>
            <div className="min-w-[200px] flex-1">
              <Input
                label="Nueva clasificación"
                value={nombre}
                onChange={(e) => setNombre(e.target.value)}
                placeholder="Ej. Electricidad"
              />
            </div>
            <Button type="submit" loading={crear.isPending} disabled={!conceptoId || !nombre.trim()}>
              Crear clasificación
            </Button>
          </form>
          {conceptos.length === 0 && (
            <p className="mt-2 text-sm text-content-muted">
              Primero creá un concepto en la pestaña anterior.
            </p>
          )}
          {crear.isError && <p className="mt-2 text-sm text-negativo">{mensajeError(crear.error)}</p>}
        </CardContent>
      </Card>

      {/* Filtros de la tabla */}
      {todas.length > 0 && (
        <div className="flex flex-wrap items-end gap-3">
          <div className="min-w-[200px]">
            <Select
              label="Filtrar por concepto"
              placeholder="Todos los conceptos"
              value={filtroConcepto}
              onChange={(e) => setFiltroConcepto(e.target.value)}
              options={conceptos.map((c) => ({ value: c.id, label: c.nombre }))}
            />
          </div>
          <div className="min-w-[220px] flex-1">
            <Input
              label="Buscar"
              value={busqueda}
              onChange={(e) => setBusqueda(e.target.value)}
              placeholder="Concepto o clasificación…"
            />
          </div>
          <p className="pb-2 text-xs text-content-muted">
            {clasificaciones.length === todas.length
              ? `${todas.length} clasificaciones`
              : `${clasificaciones.length} de ${todas.length}`}
          </p>
          {(filtroConcepto || busqueda) && (
            <Button
              variant="ghost"
              onClick={() => {
                setFiltroConcepto("");
                setBusqueda("");
              }}
            >
              Limpiar
            </Button>
          )}
        </div>
      )}

      {q.isPending ? (
        <LoadingState label="Cargando clasificaciones" />
      ) : q.isError ? (
        <ErrorState message={mensajeError(q.error)} onRetry={() => q.refetch()} />
      ) : todas.length === 0 ? (
        <EmptyState message="No hay clasificaciones registradas." />
      ) : clasificaciones.length === 0 ? (
        <EmptyState message="Ninguna clasificación coincide con el filtro." />
      ) : (
        <TableContainer>
          <Table>
            <THead>
              <TR>
                <TH>Concepto</TH>
                <TH>Clasificación</TH>
                <TH className="text-right">Acción</TH>
              </TR>
            </THead>
            <TBody>
              {clasificaciones.map((c) => (
                <TR key={c.id}>
                  <TD>
                    {editId === c.id ? (
                      <Select
                        value={editConceptoId}
                        onChange={(e) => setEditConceptoId(e.target.value)}
                        options={conceptos.map((k) => ({ value: k.id, label: k.nombre }))}
                        aria-label="Concepto de la clasificación"
                        className="min-w-[160px]"
                      />
                    ) : (
                      c.concepto
                    )}
                  </TD>
                  <TD className="font-medium">
                    {editId === c.id ? (
                      <Input
                        value={editNombre}
                        onChange={(e) => setEditNombre(e.target.value)}
                        className="max-w-xs"
                        aria-label="Nuevo nombre de la clasificación"
                      />
                    ) : (
                      c.nombre
                    )}
                  </TD>
                  <TD className="text-right">
                    {editId === c.id ? (
                      <div className="flex justify-end gap-2">
                        <Button
                          size="sm"
                          onClick={() => guardar(c)}
                          loading={renombrar.isPending || reasignar.isPending}
                          disabled={!editNombre.trim()}
                        >
                          Guardar
                        </Button>
                        <Button size="sm" variant="ghost" onClick={() => setEditId(null)}>
                          Cancelar
                        </Button>
                      </div>
                    ) : confirmandoId === c.id ? (
                      <div className="flex items-center justify-end gap-2">
                        <span className="text-xs text-content-muted">¿Eliminar «{c.nombre}»?</span>
                        <Button size="sm" variant="secondary" className="text-negativo" onClick={() => borrar(c.id)} loading={eliminar.isPending}>
                          Sí
                        </Button>
                        <Button size="sm" variant="ghost" onClick={() => setConfirmandoId(null)}>
                          No
                        </Button>
                      </div>
                    ) : (
                      <div className="flex justify-end gap-2">
                        <Button
                          size="sm"
                          variant="secondary"
                          onClick={() => {
                            setEditId(c.id);
                            setEditNombre(c.nombre);
                            setEditConceptoId(c.concepto_id);
                          }}
                        >
                          Editar
                        </Button>
                        <Button size="sm" variant="ghost" onClick={() => setFusionando(c)}>
                          Fusionar…
                        </Button>
                        <Button size="sm" variant="ghost" className="text-negativo" onClick={() => setConfirmandoId(c.id)}>
                          Eliminar
                        </Button>
                      </div>
                    )}
                  </TD>
                </TR>
              ))}
            </TBody>
          </Table>
        </TableContainer>
      )}

      {fusionando && (
        <PanelFusion
          titulo={`Fusionar «${fusionando.nombre}» con otra clasificación`}
          explicacion="Sus movimientos, reglas, documentos de CxP, vales y gastos de proveedor pasan a la clasificación destino, y esta se elimina. Si el destino está en otro concepto, los movimientos también cambian de concepto."
          opciones={clasificaciones
            .filter((x) => x.id !== fusionando.id)
            .map((x) => ({
              value: x.id,
              label: x.concepto_id === fusionando.concepto_id ? x.nombre : `${x.nombre} — ${x.concepto}`,
            }))}
          pendiente={fusionarClasif.isPending}
          avisoExtra="Si elegís una clasificación de OTRO concepto, los movimientos cambian de concepto y con eso cambia el cuadre. Se confirma con esta misma acción."
          onCancelar={() => setFusionando(null)}
          onFusionar={(destinoId) =>
            fusionarClasif.mutate(
              // El cambio de concepto va confirmado: el aviso de arriba lo dice y el usuario
              // eligió a propósito una clasificación de otro concepto en el selector.
              { id: fusionando.id, destinoId, confirmarCambioDeConcepto: true },
              {
                onSuccess: (res) => {
                  toast.success(resumenFusionTexto(res));
                  setFusionando(null);
                },
                onError: (err) => toast.error(mensajeError(err)),
              },
            )
          }
        />
      )}

      <p className="text-xs text-content-muted">
        ¿La clasificación quedó bajo el concepto equivocado (p. ej. un ingreso guardado como gasto)? Si
        todavía no tiene movimientos, usá «Editar» y cambiá el concepto. Si ya tiene movimientos, usá
        <strong> «Fusionar…»</strong>: le pasa todo a la clasificación correcta —incluso de otro concepto—
        y borra esta. No hace falta reclasificar a mano.
      </p>
    </div>
  );
}

