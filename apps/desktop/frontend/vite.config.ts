import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';

// Em desenvolvimento o painel roda em http://localhost:5173 e conversa com o
// servidor iniciado com `sacralink-server --dev` (que aceita esta origem).
export default defineConfig({
  plugins: [react(), tailwindcss()],
  build: { outDir: 'dist', emptyOutDir: true, sourcemap: false, target: 'es2022' },
  server: { port: 5173, strictPort: true },
});
