import { describe, expect, test } from "bun:test"
import { renderToStaticMarkup } from "react-dom/server"
import { getGpuUtilization } from "./systems-table-gpu-value"
import { MeterBar } from "./systems-table-meter-bar"

function renderMeter(value: number) {
	return renderToStaticMarkup(<MeterBar value={value} fillClassName="bg-green-500" />)
}

describe("MeterBar", () => {
	test("renders a measured zero as a fully empty track", () => {
		// Given / When
		const markup = renderMeter(0)

		// Then
		expect(markup).toContain('aria-valuenow="0"')
		expect(markup).toContain('style="width:0%"')
		expect(markup).not.toContain("min-w-1")
	})

	test("does not apply the zero marker to positive values", () => {
		// Given / When
		const markup = renderMeter(25)

		// Then
		expect(markup).toContain('aria-valuenow="25"')
		expect(markup).toContain('style="width:25%"')
		expect(markup).not.toContain("min-w-1")
	})
})

describe("getGpuUtilization", () => {
	test("uses measured zero when VRAM telemetry proves a GPU is present", () => {
		// Given / When
		const utilization = getGpuUtilization({ gm: 0 })

		// Then
		expect(utilization).toBe(0)
	})

	test("preserves explicit utilization and missing GPU telemetry", () => {
		// Given / When / Then
		expect(getGpuUtilization({ g: 12.5, gm: 0 })).toBe(12.5)
		expect(getGpuUtilization({})).toBeUndefined()
	})
})
