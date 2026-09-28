/**
 * Pantalla — Correo saliente (/correo-saliente).
 *
 * DESDE QUÉ BUZÓN manda cada empresa. Es el sobre; la carta (el texto de cada notificación) se
 * edita en Configuración › Notificaciones y ya era configurable por empresa.
 *
 * Tres cosas mandan el diseño de esta pantalla, y ninguna es estética:
 *
 *  1. LA CONTRASEÑA NO VUELVE NUNCA. El servidor no la devuelve —ni enmascarada—, así que el
 *     formulario no la puede round-tripear. Por eso el campo no existe hasta que se pide
 *     reemplazarla: un input vacío que al guardar borra la credencial es una trampa, no un campo.
 *  2. CAMBIAR EL SERVIDOR EXIGE REESCRIBIRLA. No es un capricho del formulario: sin esa regla,
 *     quien pueda editar esta pantalla se lleva la credencial sin verla nunca (apunta el host a su
 *     propio servidor, deja la contraseña «como está» y le da a Probar). Se avisa ANTES de
 *     guardar, porque el 422 del servidor no se arregla reintentando.
 *  3. «SIN CONFIGURAR» NO ES «NO SALE NADA». Mientras exista el buzón del grupo, los correos
 *     salen por ahí. Lo que manda es `origen_vigente`, y va arriba de todo.
 *
 * La prueba va SIEMPRE al correo del propio usuario: no hay campo «enviar a» a propósito (un
 * endpoint autenticado que manda a una dirección arbitraria desde el dominio de la empresa es
 * correo que pasa SPF y DKIM).
 */

import { useEffect, useState } from "react";
import { AlertTriangle, CheckCircle2, Mail, ShieldAlert } from "lucide-react";
import {
  Badge,
  Button,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  ErrorState,
  Input,
  LoadingState,
  PageHeader,
  Select,
  useToast,
} from "@/components/ui";
import { formatFechaHora } from "@/lib/format";
import { mensajeError } from "@/lib/apiError";
import { useAuth } from "@/features/auth/AuthContext";
import {
  SEGURIDADES,
  estadoUltimaPrueba,
  exigeReescribirPassword,
  passwordParaGuardar,
  problemasParaGuardar,
  textoOrigen,
  type FormularioCorreo,
  type ModoPassword,
} from "@/features/config/correoDominio";
import {
  useCorreoSaliente,
  useGuardarCorreoSaliente,
  useProbarCorreoSaliente,
} from "@/features/config/hooks";
import type { CorreoSaliente, SeguridadSMTP } from "@/api/correo";

/** El servidor deja probar 1 vez cada 30 s por empresa; el botón se apaga solo ese rato. */
const ESPERA_PRUEBA_S = 30;

/** El formulario que ve el usuario, a partir de lo que hay guardado. */
function formDesde(cfg: CorreoSaliente): FormularioCorreo {
  return {
    host: cfg.host,
    puerto: cfg.puerto,
    seguridad: cfg.seguridad,
    usuario: cfg.usuario,
    remitente: cfg.remitente,
    remitente_nombre: cfg.remitente_nombre,
    activo: cfg.activo,
  };
}

function igual(a: FormularioCorreo, b: FormularioCorreo): boolean {
  return (
    a.host === b.host &&
    a.puerto === b.puerto &&
    a.seguridad === b.seguridad &&
    a.usuario === b.usuario &&
    a.remitente === b.remitente &&
    a.remitente_nombre === b.remitente_nombre &&
    a.activo === b.activo
  );
}

export function CorreoSalientePage() {
  const q = useCorreoSaliente();

  return (
    <div className="flex flex-col gap-5">
      <PageHeader
        title="Correo saliente"
        description="Desde qué buzón manda esta empresa. El texto de cada correo se edita en Notificaciones."
      />
      {q.isPending ? (
        <LoadingState label="Leyendo la configuración del correo…" />
      ) : q.isError ? (
        <ErrorState message={mensajeError(q.error)} onRetry={() => void q.refetch()} />
      ) : (
        <Editor cfg={q.data} />
      )}
    </div>
  );
}

function Editor({ cfg }: { cfg: CorreoSaliente }) {
  const toast = useToast();
  const { user } = useAuth();
  const guardar = useGuardarCorreoSaliente();
  const probar = useProbarCorreoSaliente();

  const [form, setForm] = useState<FormularioCorreo>(() => formDesde(cfg));
  const [modo, setModo] = useState<ModoPassword>(cfg.tiene_password ? "conservar" : "escribir");
  const [password, setPassword] = useState("");
  /** Segundos que faltan para poder volver a probar (freno del servidor: 1 cada 30 s). */
  const [espera, setEspera] = useState(0);

  // Al volver del servidor (guardado o prueba), el formulario se sincroniza con lo guardado.
  useEffect(() => {
    setForm(formDesde(cfg));
    setModo(cfg.tiene_password ? "conservar" : "escribir");
    setPassword("");
  }, [cfg]);

  useEffect(() => {
    if (espera <= 0) return;
    const t = setTimeout(() => setEspera((s) => s - 1), 1000);
    return () => clearTimeout(t);
  }, [espera]);

  const cambiar = <K extends keyof FormularioCorreo>(campo: K, valor: FormularioCorreo[K]) =>
    setForm((f) => ({ ...f, [campo]: valor }));

  const sinCambios = igual(form, formDesde(cfg)) && modo === (cfg.tiene_password ? "conservar" : "escribir") && !password;
  const pideReescribir = exigeReescribirPassword(cfg, form);
  const problemas = problemasParaGuardar(cfg, form, modo, password);
  const estadoPrueba = estadoUltimaPrueba(cfg);

  function onGuardar() {
    if (problemas.length > 0) return;
    guardar.mutate(
      { ...form, password: passwordParaGuardar(modo, password) },
      {
        onSuccess: (res) => {
          toast.success(
            res.tiene_password
              ? "Guardado. Probá el envío para confirmar que el buzón acepta la contraseña."
              : "Guardado. El buzón queda sin autenticación.",
          );
        },
        onError: (err) => toast.error(mensajeError(err)),
      },
    );
  }

  function onProbar() {
    probar.mutate(undefined, {
      onSuccess: (res) => {
        setEspera(ESPERA_PRUEBA_S);
        toast.success(
          `Salió: ${res.descripcion}. Llegó a ${res.enviado_a} desde ${res.remitente} (${res.origen === "EMPRESA" ? "buzón de la empresa" : "buzón del grupo"}).`,
        );
      },
      onError: (err) => {
        setEspera(ESPERA_PRUEBA_S);
        toast.error(mensajeError(err));
      },
    });
  }

  return (
    <div className="flex flex-col gap-4">
      {/* De dónde sale HOY el correo. Va primero porque es lo único que contesta «¿le llega al
          proveedor?», y la respuesta no es «configurado sí/no». */}
      <div
        className={
          cfg.origen_vigente === "NINGUNO"
            ? "flex items-start gap-3 rounded-lg border border-negativo/40 bg-negativo/5 px-4 py-3"
            : cfg.origen_vigente === "GLOBAL"
              ? "flex items-start gap-3 rounded-lg border border-pendiente/40 bg-pendiente/5 px-4 py-3"
              : "flex items-start gap-3 rounded-lg border border-positivo/40 bg-positivo/5 px-4 py-3"
        }
      >
        <Mail
          className={
            cfg.origen_vigente === "NINGUNO"
              ? "mt-0.5 h-5 w-5 shrink-0 text-negativo"
              : cfg.origen_vigente === "GLOBAL"
                ? "mt-0.5 h-5 w-5 shrink-0 text-pendiente"
                : "mt-0.5 h-5 w-5 shrink-0 text-positivo"
          }
          aria-hidden
        />
        <div className="text-sm text-content">
          <p>{textoOrigen(cfg.origen_vigente, cfg.remitente_vigente)}</p>
          {cfg.configurado && !cfg.activo && (
            <p className="mt-0.5 text-content-muted">
              Esta empresa tiene su buzón configurado pero <b className="text-content">apagado</b>.
              La contraseña guardada se conserva: encenderlo no obliga a escribirla de nuevo.
            </p>
          )}
        </div>
      </div>

      {/* El servidor sin clave de cifrado no puede guardar contraseñas. Se dice ANTES de que el
          usuario escriba una y se la rebote un 422. */}
      {!cfg.cifrado_disponible && (
        <div className="flex items-start gap-3 rounded-lg border border-negativo/40 bg-negativo/5 px-4 py-3">
          <ShieldAlert className="mt-0.5 h-5 w-5 shrink-0 text-negativo" aria-hidden />
          <div className="text-sm text-content">
            <p className="font-medium">
              El servidor no tiene la clave de cifrado, así que no puede guardar contraseñas.
            </p>
            <p className="mt-0.5 text-content-muted">
              Falta la variable de entorno <code className="font-mono">CIFRADO_SECRET</code>. Se
              puede editar el resto (servidor, remitente, encendido), pero al guardar una contraseña
              el servidor responde con un error y no guarda nada.
            </p>
          </div>
        </div>
      )}

      <Card>
        <CardHeader>
          <CardTitle>Servidor de correo</CardTitle>
          <p className="mt-0.5 text-xs text-content-muted">
            Los datos que da el proveedor de correo de la empresa (Microsoft 365, Google Workspace,
            el hosting…). Son los mismos que se ponen en un cliente de correo.
          </p>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <Input
              label="Servidor (host)"
              value={form.host}
              onChange={(e) => cambiar("host", e.target.value)}
              placeholder="smtp.office365.com"
              autoComplete="off"
            />
            <div className="grid grid-cols-2 gap-3">
              <Input
                label="Puerto"
                type="number"
                min={1}
                max={65535}
                value={String(form.puerto)}
                onChange={(e) => cambiar("puerto", Number(e.target.value))}
              />
              <Select
                label="Seguridad"
                value={form.seguridad}
                onChange={(e) => cambiar("seguridad", e.target.value as SeguridadSMTP)}
                options={SEGURIDADES}
              />
            </div>
          </div>

          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <Input
              label="Usuario del buzón"
              value={form.usuario}
              onChange={(e) => cambiar("usuario", e.target.value)}
              placeholder="cxp@valledepazcr.com"
              autoComplete="off"
              hint="Vacío si el servidor no pide autenticación."
            />
            <CampoPassword
              cfg={cfg}
              modo={modo}
              password={password}
              onModo={setModo}
              onPassword={setPassword}
            />
          </div>

          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <Input
              label="Remitente (de dónde ve el proveedor que viene)"
              type="email"
              value={form.remitente}
              onChange={(e) => cambiar("remitente", e.target.value)}
              placeholder="cxp@valledepazcr.com"
              autoComplete="off"
            />
            <Input
              label="Nombre del remitente"
              value={form.remitente_nombre}
              onChange={(e) => cambiar("remitente_nombre", e.target.value)}
              placeholder="Valle de Paz — Cuentas por pagar"
            />
          </div>

          <label className="flex items-start gap-2.5 rounded-lg border border-border px-3 py-2.5">
            <input
              type="checkbox"
              checked={form.activo}
              onChange={(e) => cambiar("activo", e.target.checked)}
              className="mt-0.5 h-4 w-4 accent-accent"
            />
            <span className="text-sm text-content">
              Usar este buzón para los correos de esta empresa
              <span className="block text-xs text-content-muted">
                Apagado, la empresa vuelve a mandar por el buzón del grupo (o deja de mandar, si no
                hay). La contraseña guardada NO se borra al apagar.
              </span>
            </span>
          </label>

          {/* Por qué todavía no se puede guardar. Se dice acá, junto al botón, no en un toast
              después de intentarlo — y solo cuando hay algo escrito: una empresa sin buzón abría
              la pantalla con dos errores en rojo sin haber tocado nada. */}
          {!sinCambios && problemas.length > 0 && (
            <ul className="flex flex-col gap-1 rounded-lg border border-negativo/40 bg-negativo/10 px-3 py-2 text-xs text-content">
              {problemas.map((p) => (
                <li key={p} className="flex gap-1.5">
                  <span aria-hidden className="text-negativo">
                    •
                  </span>
                  <span>{p}</span>
                </li>
              ))}
            </ul>
          )}

          <div className="flex flex-wrap items-center gap-3">
            <Button
              onClick={onGuardar}
              loading={guardar.isPending}
              disabled={problemas.length > 0 || sinCambios}
            >
              {sinCambios ? "Sin cambios" : "Guardar"}
            </Button>
            {!sinCambios && (
              <Button
                variant="ghost"
                onClick={() => {
                  setForm(formDesde(cfg));
                  setModo(cfg.tiene_password ? "conservar" : "escribir");
                  setPassword("");
                }}
              >
                Descartar cambios
              </Button>
            )}
            {pideReescribir && (
              <span className="text-xs text-pendiente">
                Cambió el servidor del buzón: hay que escribir la contraseña de nuevo.
              </span>
            )}
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Probar el envío</CardTitle>
          <p className="mt-0.5 text-xs text-content-muted">
            Manda un correo de prueba a <b className="text-content">tu propia dirección</b>
            {user?.email ? ` (${user.email})` : ""}. No se puede elegir el destino a propósito.
            Prueba lo GUARDADO, así que primero se guarda y después se prueba.
          </p>
        </CardHeader>
        <CardContent className="flex flex-col gap-3">
          <div className="flex flex-wrap items-center gap-2.5">
            {estadoPrueba === "ok" ? (
              <Badge tone="positivo">Última prueba: salió bien</Badge>
            ) : estadoPrueba === "error" ? (
              <Badge tone="negativo">Última prueba: falló</Badge>
            ) : (
              <Badge tone="neutral">Nunca se probó</Badge>
            )}
            {cfg.probado_en && (
              <span className="text-xs text-content-muted">{formatFechaHora(cfg.probado_en)}</span>
            )}
          </div>

          {/* El ÚNICO lugar del sistema con el detalle técnico del fallo (categoría, código SMTP y
              servidor). Se lee con admin.correo; la bitácora de CxP, que ven siete roles, lleva
              solo la frase para el operador. */}
          {cfg.probado_error && (
            <div className="flex items-start gap-2.5 rounded-lg border border-negativo/40 bg-negativo/5 px-3 py-2">
              <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-negativo" aria-hidden />
              <p className="text-xs text-content">{cfg.probado_error}</p>
            </div>
          )}
          {estadoPrueba === "ok" && (
            <div className="flex items-start gap-2.5 text-xs text-content-muted">
              <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0 text-positivo" aria-hidden />
              <p>El buzón aceptó la conexión y la credencial la última vez que se probó.</p>
            </div>
          )}

          <div className="flex flex-wrap items-center gap-3">
            <Button
              variant="secondary"
              onClick={onProbar}
              loading={probar.isPending}
              disabled={espera > 0 || !sinCambios}
            >
              {espera > 0 ? `Probar (esperá ${espera} s)` : "Enviarme un correo de prueba"}
            </Button>
            {!sinCambios && (
              <span className="text-xs text-content-muted">
                Guardá los cambios primero: la prueba usa lo que está guardado, no lo que dice el
                formulario.
              </span>
            )}
          </div>
        </CardContent>
      </Card>
    </div>
  );
}

/**
 * El campo de contraseña, con los tres estados del contrato.
 *
 * Cuando hay una guardada NO se pinta un input: se dice que existe y se ofrece reemplazarla. Un
 * campo vacío que al guardar borra la credencial es una trampa —y el servidor, que no la devuelve,
 * no tiene forma de volver a llenarlo—.
 */
function CampoPassword({
  cfg,
  modo,
  password,
  onModo,
  onPassword,
}: {
  cfg: CorreoSaliente;
  modo: ModoPassword;
  password: string;
  onModo: (m: ModoPassword) => void;
  onPassword: (v: string) => void;
}) {
  if (!cfg.tiene_password) {
    return (
      <Input
        label="Contraseña del buzón"
        type="password"
        value={password}
        onChange={(e) => onPassword(e.target.value)}
        autoComplete="new-password"
        hint="Se guarda cifrada. No se vuelve a mostrar nunca, ni acá ni en un error."
      />
    );
  }

  if (modo === "escribir") {
    return (
      <div className="flex flex-col gap-1.5">
        <Input
          label="Contraseña nueva del buzón"
          type="password"
          value={password}
          onChange={(e) => onPassword(e.target.value)}
          autoComplete="new-password"
          hint="Reemplaza a la guardada. Se cifra al guardar y no se vuelve a mostrar."
        />
        <button
          type="button"
          className="self-start text-xs text-content-muted underline hover:text-content"
          onClick={() => {
            onPassword("");
            onModo("conservar");
          }}
        >
          Cancelar el reemplazo (dejar la contraseña guardada)
        </button>
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-1.5">
      <span className="text-sm font-medium text-content">Contraseña del buzón</span>
      <div className="flex h-10 items-center gap-2 rounded-lg border border-border bg-surface-muted px-3">
        <Badge tone={modo === "quitar" ? "negativo" : "positivo"}>
          {modo === "quitar" ? "Se va a borrar" : "Guardada"}
        </Badge>
        <span className="truncate text-xs text-content-muted">
          {modo === "quitar"
            ? "Al guardar, el buzón queda sin autenticación."
            : "No se puede mostrar: se guarda cifrada."}
        </span>
      </div>
      <div className="flex flex-wrap gap-3">
        {modo === "quitar" ? (
          <button
            type="button"
            className="text-xs text-content-muted underline hover:text-content"
            onClick={() => onModo("conservar")}
          >
            No borrarla
          </button>
        ) : (
          <>
            <button
              type="button"
              className="text-xs text-accent underline"
              onClick={() => onModo("escribir")}
            >
              Reemplazar la contraseña
            </button>
            <button
              type="button"
              className="text-xs text-content-muted underline hover:text-content"
              onClick={() => onModo("quitar")}
            >
              Quitarla (buzón sin autenticación)
            </button>
          </>
        )}
      </div>
    </div>
  );
}
