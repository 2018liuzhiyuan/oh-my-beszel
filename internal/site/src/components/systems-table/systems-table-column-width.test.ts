import { describe, expect, test } from "bun:test"
import { distributeColumnWidths, getContentAwareColumnWidth } from "./content-aware-column-width"

describe("getContentAwareColumnWidth", () => {
	test("grows a text column for longer content", () => {
		// Given
		const compact = {
			header: "System",
			values: ["gpu-node-a"],
			min: 112,
			max: 320,
			headerControls: 76,
			cellPadding: 36,
		}
		const expanded = { ...compact, values: ["production-gpu-worker-12"] }

		// When
		const compactWidth = getContentAwareColumnWidth(compact)
		const expandedWidth = getContentAwareColumnWidth(expanded)

		// Then
		expect(expandedWidth).toBeGreaterThan(compactWidth)
	})

	test("caps pathological content and rounds widths to the spacing grid", () => {
		// Given
		const request = {
			header: "System",
			values: ["x".repeat(200)],
			min: 112,
			max: 320,
			headerControls: 76,
			cellPadding: 36,
		}

		// When
		const width = getContentAwareColumnWidth(request)

		// Then
		expect(width).toBe(320)
		expect(width % 4).toBe(0)
	})

	test("reserves full-width space for CJK labels", () => {
		// Given
		const base = { values: [], min: 64, max: 240, headerControls: 76, cellPadding: 36 }

		// When
		const asciiWidth = getContentAwareColumnWidth({ ...base, header: "ABCDEF" })
		const cjkWidth = getContentAwareColumnWidth({ ...base, header: "正常运行时间" })

		// Then
		expect(cjkWidth).toBeGreaterThan(asciiWidth)
	})
})

describe("distributeColumnWidths", () => {
	test("fills wide viewports proportionally without stretching only the system column", () => {
		const widths = distributeColumnWidths({ system: 108, gpu: 116, vram: 108, actions: 80 }, 800, new Set(["actions"]))

		expect(Object.values(widths).reduce((total, width) => total + width, 0)).toBe(800)
		expect(widths.actions).toBe(80)
		expect(widths.system).toBeLessThan(300)
		expect(widths.gpu).toBeGreaterThan(widths.system)
	})

	test("keeps intrinsic widths when the viewport is narrower than the table", () => {
		const widths = { system: 108, gpu: 116, actions: 80 }
		expect(distributeColumnWidths(widths, 240, new Set(["actions"]))).toEqual(widths)
	})
})
