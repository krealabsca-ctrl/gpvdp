import { RouterProvider } from "react-router-dom";
import { AuthProvider } from "@/features/auth/AuthContext";
import { ToastProvider } from "@/components/ui";
import { EmpresaTheme } from "@/components/shell/EmpresaTheme";
import { router } from "@/routes/router";

/**
 * App raíz. El AuthProvider envuelve al router para que los guards y páginas
 * accedan al estado de auth. El ToastProvider expone useToast() a todo el árbol.
 * El QueryClientProvider se monta en main.tsx.
 */
export default function App() {
  return (
    <AuthProvider>
      <EmpresaTheme />
      <ToastProvider>
        <RouterProvider router={router} />
      </ToastProvider>
    </AuthProvider>
  );
}
