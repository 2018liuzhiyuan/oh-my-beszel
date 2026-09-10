import { describe, expect, test } from "bun:test"
import type { GPUData, SystemStats, SystemStatsRecord } from "@/types"
import { buildGpuSummaryData } from "./gpu-summary-data"

const baseStats = {
	cpu: 0,
	m: 0,
	mu: 0,
	mp: 0,
	mb: 0,
	s: 0,
	su: 0,
	d: 0,
	du: 0,
	dp: 0,
	dr: 0,
	dw: 0,
	ns: 0,
	nr: 0,
} satisfies SystemStats

function createStatsRecord(stats: SystemStats, id = "sample"): SystemStatsRecord {
	return {
		id,
		collectionId: "system_stats",
		collectionName: "system_stats",
		system: "system",
		created: 1,
		stats,
	}
}

const latestGpus: Record<string, GPUData> = {
	"1": { n: "A800", u: 20, mu: 20_000, mt: 80_000 },
	"0": { n: "A800", u: 10, mu: 10_000, mt: 40_000 },
	"2": { n: "Display", u: 5, mt: 0 },
}

const sample = createStatsRecord({
	...baseStats,
	g: {
		"0": { n: "A800", u: 35, mu: 12_000, mt: 40_000 },
		"1": { n: "A800", u: 65, mu: 24_000, mt: 80_000 },
		"2": { n: "Display", u: 15 },
	},
})

describe("buildGpuSummaryData", () => {
	test("returns an empty summary when no GPUs are available", () => {
		// Given
		const gpus: Record<string, GPUData> = {}

		// When
		const summary = buildGpuSummaryData(gpus)

		// Then
		expect(summary).toEqual({ usage: [], vram: [], vramMax: 0 })
	})

	test("builds deterministic unique series for every GPU", () => {
		// Given
		const gpus = latestGpus

		// When
		const summary = buildGpuSummaryData(gpus)

		// Then
		expect(summary.usage.map(({ label }) => label)).toEqual(["A800 0", "A800 1", "Display"])
		expect(summary.usage.map(({ dataKey }) => dataKey(sample))).toEqual([35, 65, 15])
	})

	test("orders GPU IDs with numeric suffixes naturally", () => {
		// Given
		const gpus: Record<string, GPUData> = {
			"gpu-10": { n: "A800", u: 10 },
			"gpu-2": { n: "A800", u: 20 },
			"gpu-1": { n: "A800", u: 30 },
		}

		// When
		const summary = buildGpuSummaryData(gpus)

		// Then
		expect(summary.usage.map(({ label }) => label)).toEqual(["A800 gpu-1", "A800 gpu-2", "A800 gpu-10"])
	})

	test("keeps labels and colors stable across input insertion orders", () => {
		// Given
		const firstOrder: Record<string, GPUData> = {
			"gpu-10": { n: "A800", u: 10 },
			"gpu-2": { n: "Display", u: 20 },
		}
		const secondOrder: Record<string, GPUData> = {
			"gpu-2": { n: "Display", u: 20 },
			"gpu-10": { n: "A800", u: 10 },
		}

		// When
		const firstSummary = buildGpuSummaryData(firstOrder)
		const secondSummary = buildGpuSummaryData(secondOrder)

		// Then
		expect(firstSummary.usage.map(({ label, color }) => ({ label, color }))).toEqual(
			secondSummary.usage.map(({ label, color }) => ({ label, color }))
		)
	})

	test("does not mutate the source GPU telemetry", () => {
		// Given
		const gpus = structuredClone(latestGpus)
		const original = structuredClone(gpus)

		// When
		buildGpuSummaryData(gpus)

		// Then
		expect(gpus).toEqual(original)
	})

	test("includes VRAM only for GPUs with capacity and exposes the largest capacity", () => {
		// Given
		const gpus = latestGpus

		// When
		const summary = buildGpuSummaryData(gpus)

		// Then
		expect(summary.vram.map(({ label }) => label)).toEqual(["A800 0", "A800 1"])
		expect(summary.vram.map(({ dataKey }) => dataKey(sample))).toEqual([12_000, 24_000])
		expect(summary.vramMax).toBe(80_000)
	})

	test("renders missing samples as measured zero", () => {
		// Given
		const emptySample = createStatsRecord(baseStats, "empty")

		// When
		const summary = buildGpuSummaryData(latestGpus)

		// Then
		expect(summary.usage[0]?.dataKey(emptySample)).toBe(0)
		expect(summary.vram[0]?.dataKey(emptySample)).toBe(0)
	})
})
