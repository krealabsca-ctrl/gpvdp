/**
 * CxP — Nuevo documento (/cxp/documentos/nuevo).
 * Alta manual de una factura/comprobante de proveedor (más adelante también por
 * ingesta XML 4.4). La Clave es la llave anti-duplicado (Hacienda 4.4). Si la
 * moneda es USD, el TC es obligatorio (el backend calcula total_crc = total × tc).
 */

import { useState, type FormEvent } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import {
  Button,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  Input,
  PageHeader,
  Select,
  useToast,
} from "@/components/ui";
import { mensajeError } from "@/lib/apiError";
import { useCrearDocumento, useTodosProveedores } from "@/features/cxp/hooks";
import type { DocumentoInput } from "@/api/cxp";
import {
  centimosALegible,
  centimosAPlano,
  formatMoneda,
  montoACentimos,
  montoLegible,
  montoParaApi,
  type Moneda,
} from "@/lib/format";

const MONEDA_OPTIONS = [
  { value: "CRC", label: "Colones (CRC)" },
  { value: "USD", label: "Dólares (USD)" },
];

const TIPO_OPTIONS = [
  { value: "CXP", label: "Factura electrónica (CxP)" },
  { value: "ANTICIPO", label: "Anticipo / adelanto" },
  { value: "INTERNO", label: "Interno (liquidación / arreglo / negociación)" },
  { value: "VIATICOS", label: "Viáticos" },
  { value: "REINTEGRO", label: "Reintegro" },
];

// Tarifas de IVA vigentes en CR: 13% general + reducidas. Parametrizable por documento
// (no se fija en código) porque la tarifa depende del bien/servicio facturado.
const IVA_OPTIONS = [
  { value: "13", label: "13% (general)" },
  { value: "4", label: "4% (reducida)" },
  { value: "2", label: "2% (reducida)" },
  { value: "1", label: "1% (reducida)" },
  { value: "0", label: "0% (exento / no aplica)" },
];

// Aritmética en céntimos (sin arrastre de punto flotante). El parseo/formateo vive en lib/format
// para que sea el mismo en todo CxP: se MUESTRA con separador de miles y se ENVÍA decimal plano.
const aCent = montoACentimos;
const deCent = centimosALegible;
/** IVA sobre la base imponible (subtotal). */
function ivaDeSubtotal(subCent: number, tasa: number): number {
  return Math.round((subCent * tasa) / 100);
}
/** Desglose inverso: del total con IVA se despeja la base (sub + iva === total exacto). */
function subtotalDeTotal(totalCent: number, tasa: number): number {
  return Math.round(totalCent / (1 + tasa / 100));
}

export function NuevoDocumentoPage() {
  const toast = useToast();
  const navigate = useNavigate();
  const proveedoresQuery = useTodosProveedores();
  const crear = useCrearDocumento();

  // ?tipo=ANTICIPO preselecciona el tipo (atajo «Nuevo anticipo» desde la página de Anticipos).
  const [params] = useSearchParams();
  const tipoInicial = TIPO_OPTIONS.some((t) => t.value === params.get("tipo")) ? params.get("tipo")! : "CXP";

  const [proveedorId, setProveedorId] = useState("");
  const [tipo, setTipo] = useState(tipoInicial);
  const [clave, setClave] = useState("");
  const [consecutivo, setConsecutivo] = useState("");
  const [fechaEmision, setFechaEmision] = useState("");
  const [moneda, setMoneda] = useState<Moneda>("CRC");
  const [subtotal, setSubtotal] = useState("");
  const [iva, setIva] = useState("");
  const [retencion, setRetencion] = useState("");
  const [total, setTotal] = useState("");
  const [tasaIva, setTasaIva] = useState(tipoInicial === "CXP" ? "13" : "0");
  const [tc, setTc] = useState("");
  const [vencimiento, setVencimiento] = useState("");
  const [descripcion, setDescripcion] = useState("");

  const proveedores = proveedoresQuery.data ?? [];
  const proveedorOptions = proveedores.filter((p) => p.activo).map((p) => ({ value: p.id, label: p.nombre }));

  const esElectronica = tipo === "CXP";
  // Vía expresa: la solicitud de anticipo es corta — monto + motivo/respaldo. Sin desglose de
  // IVA/retención (no es el gasto; el gasto se clasifica en la factura final).
  const esAnticipo = tipo === "ANTICIPO";
  const proveedor = proveedores.find((p) => p.id === proveedorId);
  // Retención de renta en la fuente: % configurado en el proveedor, aplicado sobre la base (subtotal).
  const pctRetencion = Number(proveedor?.retencion_renta_pct ?? "0") || 0;
  const tasa = Number(tasaIva) || 0;

  // El campo que tocás manda: al escribir el subtotal se derivan IVA y total; al escribir el
  // total se desglosa hacia atrás. IVA y retención siguen siendo editables a mano.
  function aplicarDesdeSubtotal(sub: string, t = tasa, pct = pctRetencion) {
    setSubtotal(sub);
    const subC = aCent(sub);
    const ivaC = ivaDeSubtotal(subC, t);
    setIva(deCent(ivaC));
    setTotal(deCent(subC + ivaC));
    setRetencion(deCent(Math.round((subC * pct) / 100)));
  }
  function aplicarDesdeTotal(tot: string, t = tasa, pct = pctRetencion) {
    setTotal(tot);
    const totC = aCent(tot);
    const subC = subtotalDeTotal(totC, t);
    setSubtotal(deCent(subC));
    setIva(deCent(totC - subC));
    setRetencion(deCent(Math.round((subC * pct) / 100)));
  }
  function cambiarTasa(nueva: string) {
    setTasaIva(nueva);
    const t = Number(nueva) || 0;
    if (subtotal.trim()) aplicarDesdeSubtotal(subtotal, t);
    else if (total.trim()) aplicarDesdeTotal(total, t);
  }
  function cambiarProveedor(id: string) {
    setProveedorId(id);
    const p = proveedores.find((x) => x.id === id);
    const pct = Number(p?.retencion_renta_pct ?? "0") || 0;
    // Proveedor exento de IVA → tarifa 0 (se puede cambiar a mano si el caso lo amerita).
    const t = p?.exento_iva ? 0 : tasa;
    if (p?.exento_iva) setTasaIva("0");
    if (subtotal.trim()) aplicarDesdeSubtotal(subtotal, t, pct);
    else if (total.trim()) aplicarDesdeTotal(total, t, pct);
  }
  function cambiarTipo(nuevo: string) {
    setTipo(nuevo);
    // Un anticipo/documento interno normalmente no desglosa IVA: arranca en 0% (editable)…
    if (nuevo !== "CXP" && tasaIva === "13") cambiarTasa("0");
    // …y al volver a factura electrónica se restaura la tarifa general, para no registrar
    // por descuido una factura sin IVA (salvo proveedor exento).
    if (nuevo === "CXP" && tasaIva === "0" && !proveedor?.exento_iva) cambiarTasa("13");
  }

  // Lo que realmente se le transfiere al proveedor (espejo del archivo de pago del banco).
  // En plano porque formatMoneda espera decimal, no texto ya localizado.
  const netoAPagar = centimosAPlano(Math.max(0, aCent(total) - aCent(retencion)));

  function onSubmit(e: FormEvent) {
    e.preventDefault();
    if (!proveedorId) return toast.error("Elegí un proveedor.");
    if (esElectronica && !clave.trim())
      return toast.error("La clave de Hacienda es obligatoria para una factura electrónica.");
    if (!fechaEmision) return toast.error("La fecha de emisión es obligatoria.");
    if (!total.trim()) return toast.error("El total es obligatorio.");
    if (moneda === "USD" && !tc.trim()) return toast.error("Para USD, el tipo de cambio es obligatorio.");
    if (esAnticipo && !descripcion.trim())
      return toast.error("El anticipo requiere el motivo y su respaldo (cotización/contrato).");

    const input: DocumentoInput = {
      proveedor_id: proveedorId,
      tipo,
      clave: esElectronica ? clave.trim() : "",
      consecutivo: consecutivo.trim() || undefined,
      fecha_emision: fechaEmision,
      moneda,
      // A la API van SIEMPRE en decimal plano (el campo se muestra con separador de miles).
      subtotal: montoParaApi(subtotal) || undefined,
      iva: montoParaApi(iva) || undefined,
      retencion: montoParaApi(retencion) || undefined,
      total: centimosAPlano(aCent(total)),
      tc: moneda === "USD" ? tc.trim() : undefined,
      fecha_vencimiento: vencimiento || undefined,
      descripcion: descripcion.trim() || undefined,
    };

    crear.mutate(input, {
      onSuccess: (doc) => {
        toast.success("Documento creado.");
        navigate(`/cxp/documentos/${doc.id}`);
      },
      onError: (err) => toast.error(mensajeError(err)),
    });
  }

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title="Nuevo documento"
        description="Registrá una factura electrónica, un anticipo/adelanto o un documento interno (sin factura electrónica). La clave de Hacienda solo aplica a la factura electrónica."
        actions={
          <Button variant="secondary" onClick={() => navigate("/cxp/documentos")}>
            Volver
          </Button>
        }
      />

      <Card>
        <CardHeader>
          <CardTitle>Datos del comprobante</CardTitle>
        </CardHeader>
        <CardContent>
          <form onSubmit={onSubmit} className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
            <Select
              label="Tipo de documento *"
              value={tipo}
              onChange={(e) => cambiarTipo(e.target.value)}
              options={TIPO_OPTIONS}
            />
            <Select
              label="Proveedor *"
              placeholder={proveedoresQuery.isPending ? "Cargando…" : "Seleccioná…"}
              value={proveedorId}
              onChange={(e) => cambiarProveedor(e.target.value)}
              options={proveedorOptions}
            />
            {esElectronica ? (
              <Input
                label="Clave (50 díg.) *"
                value={clave}
                onChange={(e) => setClave(e.target.value)}
                placeholder="Clave del comprobante 4.4"
                className="font-mono"
              />
            ) : (
              <div className="sm:col-span-2">
                <Input
                  label="Referencia (opcional)"
                  value={clave}
                  onChange={(e) => setClave(e.target.value)}
                  placeholder="Sin factura electrónica — dejala vacía y se genera una referencia interna"
                  className="font-mono"
                />
              </div>
            )}
            <Input
              label="Consecutivo"
              value={consecutivo}
              onChange={(e) => setConsecutivo(e.target.value)}
              placeholder="00100001010000000001"
              className="font-mono"
            />
            <Input
              label="Fecha de emisión *"
              type="date"
              value={fechaEmision}
              onChange={(e) => setFechaEmision(e.target.value)}
            />
            <Input
              label="Fecha de vencimiento"
              type="date"
              value={vencimiento}
              onChange={(e) => setVencimiento(e.target.value)}
            />
            <Select
              label="Moneda *"
              value={moneda}
              onChange={(e) => setMoneda(e.target.value as Moneda)}
              options={MONEDA_OPTIONS}
            />
            {moneda === "USD" && (
              <Input
                label="Tipo de cambio *"
                value={tc}
                onChange={(e) => setTc(e.target.value)}
                placeholder="Ej. 515.25"
                inputMode="decimal"
              />
            )}
            {!esAnticipo && (
              <>
                <Select
                  label="Tarifa de IVA"
                  value={tasaIva}
                  onChange={(e) => cambiarTasa(e.target.value)}
                  options={IVA_OPTIONS}
                />
                <Input
                  label="Subtotal"
                  value={subtotal}
                  onChange={(e) => aplicarDesdeSubtotal(e.target.value)}
                  onBlur={() => setSubtotal(montoLegible(subtotal))}
                  placeholder="0,00"
                  inputMode="decimal"
                  className="text-right tabular-nums"
                />
                <Input
                  label="IVA"
                  value={iva}
                  onChange={(e) => {
                    // IVA a mano: manda sobre la tarifa y recalcula el total (subtotal intacto).
                    setIva(e.target.value);
                    setTotal(deCent(aCent(subtotal) + aCent(e.target.value)));
                  }}
                  onBlur={() => setIva(montoLegible(iva))}
                  placeholder="0,00"
                  inputMode="decimal"
                  className="text-right tabular-nums"
                />
              </>
            )}
            <Input
              label={esAnticipo ? "Monto del anticipo *" : "Total *"}
              value={total}
              onChange={(e) => aplicarDesdeTotal(e.target.value)}
              onBlur={() => setTotal(montoLegible(total))}
              placeholder="0,00"
              inputMode="decimal"
              className="text-right tabular-nums"
            />
            {!esAnticipo && (
              <>
                <Input
                  label={pctRetencion > 0 ? `Retención (${pctRetencion}% del proveedor)` : "Retención"}
                  value={retencion}
                  onChange={(e) => setRetencion(e.target.value)}
                  onBlur={() => setRetencion(montoLegible(retencion))}
                  placeholder="0,00"
                  inputMode="decimal"
                  className="text-right tabular-nums"
                />
                <div className="flex items-end pb-1 sm:col-span-2">
                  <p className="text-sm text-content-muted">
                    Neto a pagar al proveedor:{" "}
                    <span className="font-semibold tabular-nums text-accent">
                      {formatMoneda(netoAPagar, moneda)}
                    </span>
                    {aCent(retencion) > 0 && " (total − retención)"}
                  </p>
                </div>
              </>
            )}
            <div className="sm:col-span-2 lg:col-span-3">
              <Input
                label={esAnticipo ? "Motivo y respaldo (cotización / contrato) *" : "Descripción"}
                value={descripcion}
                onChange={(e) => setDescripcion(e.target.value)}
                placeholder={
                  esAnticipo
                    ? "Ej. Adelanto repuesto carroza — cotización #123 de Taller X"
                    : "Detalle / referencia interna"
                }
              />
            </div>
            {esAnticipo && (
              <div className="sm:col-span-2 lg:col-span-3">
                <p className="rounded-lg border border-border bg-surface-muted px-3 py-2 text-xs text-content-muted">
                  Vía expresa: el anticipo no pasa por la validación de área ni clasifica gasto — se aprueba
                  directo con la matriz de firmas y se paga. El gasto se clasifica una sola vez, en la factura
                  final, donde también se descuenta este adelanto.
                </p>
              </div>
            )}
            <div className="flex items-end">
              <Button
                type="submit"
                loading={crear.isPending}
                disabled={!proveedorId || (esElectronica && !clave.trim()) || (esAnticipo && !descripcion.trim())}
              >
                Crear documento
              </Button>
            </div>
          </form>
        </CardContent>
      </Card>
    </div>
  );
}
