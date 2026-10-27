import { defineConfig } from "vite";
export default defineConfig({
  base: "./",
  esbuild: { jsxFactory: "React.createElement", jsxFragment: "React.Fragment" },
  build: {
    outDir: "build",
    rollupOptions: {
      preserveEntrySignatures: "strict",
      input: "src/plugin.jsx",
      output: { entryFileNames: "assets/plugin.js" },
    },
  },
});
