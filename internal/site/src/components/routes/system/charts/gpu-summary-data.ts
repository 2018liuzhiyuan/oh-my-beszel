import type { DataPoint } from "@/components/charts/line-chart"
import type { GPUData } from "@/types"

export type GpuSummaryData = {
	readonly usage: DataPoint[]
	readonly vram: DataPoint[]
	readonly vramMax: number
}

export function buildGpuSummaryData(gpus: Record<string, GPUData>): GpuSummaryData {
	const entries = Object.entries(gpus).sort(([leftId], [rightId]) =>
		leftId.localeCompare(rightId, "en", { numeric: true })
	)
	const nameCounts = new Map<string, number>()
	for (const [, gpu] of entries) {
		nameCounts.set(gpu.n, (nameCounts.get(gpu.n) ?? 0) + 1)
	}
	const series = entries.map(([id, gpu], index) => ({
		id,
		gpu,
		label: (nameCounts.get(gpu.n) ?? 0) > 1 ? `${gpu.n} ${id}` : gpu.n,
		color: `hsl(${226 + (((index * 360) / entries.length) % 360)}, 65%, 52%)`,
	}))

	return {
		usage: series.map(({ id, label, color }) => ({
			label,
			dataKey: ({ stats }) => stats?.g?.[id]?.u ?? 0,
			color,
		})),
		vram: series
			.filter(({ gpu }) => (gpu.mt ?? 0) > 0)
			.map(({ id, label, color }) => ({
				label,
				dataKey: ({ stats }) => stats?.g?.[id]?.mu ?? 0,
				color,
			})),
		vramMax: Math.max(0, ...series.map(({ gpu }) => gpu.mt ?? 0)),
	}
}
