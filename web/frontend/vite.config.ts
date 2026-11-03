import { fileURLToPath, URL } from "node:url";
import { defineConfig } from "vite";
import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';

// 构建产物输出到 web/dist，由 go:embed 打进二进制。
// dev 模式通过 proxy 访问本地 Go 后端，前后端分离开发。
export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      "@": fileURLToPath(new URL("./src", import.meta.url)),
    },
  },
  build: {
    outDir: '../dist',
    emptyOutDir: true,
  },
  server: {
    port: 5173,
    proxy: {
      '/api': 'http://localhost:8090',
    },
  },
});
