import { defineConfig } from "vite"
import path from "node:path"
import tailwindcss from "@tailwindcss/vite"
import babel from "@rolldown/plugin-babel"
import react from "@vitejs/plugin-react"
import { lingui, linguiTransformerBabelPreset } from "@lingui/vite-plugin"

export default defineConfig({
	base: "./",
	plugins: [
		react(),
		lingui(),
		babel({
			presets: [linguiTransformerBabelPreset()],
		}),
		tailwindcss(),
	],
	build: {
		license: true,
	},
	resolve: {
		alias: {
			"@": path.resolve(import.meta.dirname, "./src"),
		},
	},
})
