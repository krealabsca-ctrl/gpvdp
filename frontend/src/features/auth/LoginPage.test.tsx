import { describe, it, expect, beforeEach, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter, Routes, Route } from "react-router-dom";

// Mock del cliente AUTH: los tests NO tocan la red.
vi.mock("@/api/auth", () => ({
  authApi: {
    login: vi.fn(),
    selectEmpresa: vi.fn(),
    me: vi.fn(),
    empresas: vi.fn(),
    healthz: vi.fn(),
  },
}));

import { authApi } from "@/api/auth";
import { AuthProvider } from "@/features/auth/AuthContext";
import { LoginPage } from "@/features/auth/LoginPage";
import type { LoginResponse } from "@/api/types";

// Token JWT falso con claim empresa_id (payload base64url) para el flujo de 1 empresa.
// header.payload.signature — payload = {"empresa_id":"emp-1"}
const TOKEN_CON_EMPRESA =
  "eyJhbGciOiJIUzI1NiJ9.eyJlbXByZXNhX2lkIjoiZW1wLTEifQ.sig";

function renderLogin() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <AuthProvider>
        <MemoryRouter initialEntries={["/login"]}>
          <Routes>
            <Route path="/login" element={<LoginPage />} />
            <Route path="/" element={<div>Dashboard OK</div>} />
            <Route path="/seleccionar-empresa" element={<div>Selector OK</div>} />
          </Routes>
        </MemoryRouter>
      </AuthProvider>
    </QueryClientProvider>,
  );
}

describe("LoginPage", () => {
  beforeEach(() => {
    localStorage.clear();
    vi.clearAllMocks();
  });

  it("renderiza los campos de correo y contraseña", () => {
    renderLogin();
    expect(screen.getByLabelText(/correo/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/contraseña/i)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /entrar/i })).toBeInTheDocument();
  });

  it("con credenciales válidas y una sola empresa, auto-selecciona y entra", async () => {
    const loginResp: LoginResponse = {
      access_token: "tok-sin-empresa",
      refresh_token: "refresh-1",
      user: { id: "u1", nombre: "Ana", email: "ana@vdp.com" },
      empresas: [{ id: "emp-1", nombre: "Valle de Paz", rol: "ADMIN" }],
    };
    vi.mocked(authApi.login).mockResolvedValue(loginResp);
    vi.mocked(authApi.selectEmpresa).mockResolvedValue({
      access_token: TOKEN_CON_EMPRESA,
    });

    const user = userEvent.setup();
    renderLogin();

    await user.type(screen.getByLabelText(/correo/i), "ana@vdp.com");
    await user.type(screen.getByLabelText(/contraseña/i), "secreto123");
    await user.click(screen.getByRole("button", { name: /entrar/i }));

    // Auto-selección de la única empresa -> dashboard.
    await waitFor(() => {
      expect(authApi.login).toHaveBeenCalledWith("ana@vdp.com", "secreto123");
      expect(authApi.selectEmpresa).toHaveBeenCalledWith("emp-1");
      expect(screen.getByText("Dashboard OK")).toBeInTheDocument();
    });
  });

  it("muestra un mensaje de error cuando el login falla", async () => {
    const { ApiError } = await import("@/api/client");
    vi.mocked(authApi.login).mockRejectedValue(
      new ApiError(401, "INVALID_CREDENTIALS", "credenciales"),
    );

    const user = userEvent.setup();
    renderLogin();

    await user.type(screen.getByLabelText(/correo/i), "ana@vdp.com");
    await user.type(screen.getByLabelText(/contraseña/i), "malas");
    await user.click(screen.getByRole("button", { name: /entrar/i }));

    await waitFor(() => {
      expect(screen.getByRole("alert")).toHaveTextContent(/credenciales inválidas/i);
    });
  });
});
