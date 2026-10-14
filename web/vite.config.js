import { resolve } from "node:path";
import tailwindcss from "@tailwindcss/vite";
import { defineConfig, loadEnv } from "vite";

// The API allows the development origin with cors_origin in oai.toml (OAI_CORS_ORIGIN).
const devPort = Number(process.env.OAI_WEB_PORT || 8000);
if (!Number.isInteger(devPort) || devPort < 1 || devPort > 65535) {
  throw new Error(`OAI_WEB_PORT is not a port: ${process.env.OAI_WEB_PORT}`);
}

const config = {
  plugins: [tailwindcss()],
  server: { port: devPort, strictPort: true },
  preview: { port: devPort, strictPort: true },
  build: {
    rollupOptions: {
      input: {
        search: resolve(import.meta.dirname, "index.html"),
        agent: resolve(import.meta.dirname, "agent.html"),
      },
    },
  },
};

export default defineConfig(({ mode }) => {
  if (!loadEnv(mode, import.meta.dirname).VITE_API_URL) {
    throw new Error("Set VITE_API_URL to the API base URL, for example https://api.example.com");
  }
  return config;
});
