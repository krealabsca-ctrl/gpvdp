import { useState } from "react";
import { useAuth } from "@/features/auth/AuthContext";
import { Select } from "@/components/ui";

/**
 * Dropdown para cambiar de empresa activa.
 * Al elegir, re-llama select-empresa (nuevo token scopeado) y segrega la caché.
 * Si el usuario solo pertenece a una empresa, se muestra como etiqueta estática.
 */
export function EmpresaSwitcher() {
  const { empresas, empresaActiva, selectEmpresa } = useAuth();
  const [cambiando, setCambiando] = useState(false);

  // Con una sola empresa no hay nada que elegir.
  if (empresas.length <= 1) {
    return (
      <span className="text-sm font-medium text-content" aria-label="Empresa activa">
        {empresaActiva?.nombre ?? "—"}
      </span>
    );
  }

  async function onChange(nextId: string) {
    if (!nextId || nextId === empresaActiva?.id) return;
    setCambiando(true);
    try {
      await selectEmpresa(nextId);
    } finally {
      setCambiando(false);
    }
  }

  return (
    <Select
      aria-label="Cambiar empresa activa"
      className="h-9 w-48"
      value={empresaActiva?.id ?? ""}
      disabled={cambiando}
      onChange={(e) => onChange(e.target.value)}
      options={empresas.map((e) => ({ value: e.id, label: e.nombre }))}
      placeholder={empresaActiva ? undefined : "Seleccionar empresa"}
    />
  );
}
