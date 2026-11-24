import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider } from "@tanstack/react-router";
import { TooltipProvider } from "@/components/ui/tooltip";
import { AuthKitProvider } from "@/features/auth/authkit-provider";
import { router } from "./routes";
import "@fontsource-variable/geist";
import "@fontsource-variable/geist-mono";
import "@/styles.css";
const client = new QueryClient({
  defaultOptions: {
    queries: { staleTime: 5000, retry: 1, refetchOnWindowFocus: true },
    mutations: { retry: false },
  },
});
createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <QueryClientProvider client={client}>
      <TooltipProvider>
        <AuthKitProvider>
          <RouterProvider router={router} />
        </AuthKitProvider>
      </TooltipProvider>
    </QueryClientProvider>
  </StrictMode>,
);
