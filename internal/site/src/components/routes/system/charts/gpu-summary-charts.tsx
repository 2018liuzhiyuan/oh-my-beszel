import { t } from "@lingui/core/macro"
import { useMemo } from "react"
import LineChartDefault from "@/components/charts/line-chart"
import { Unit } from "@/lib/enums"
import { decimalString, formatBytes, toFixedFloat } from "@/lib/utils"
import type { ChartData, GPUData } from "@/types"
import { ChartCard } from "../chart-card"
import { buildGpuSummaryData } from "./gpu-summary-data"

type GpuSummaryChartsProps = {
	readonly chartData: ChartData
	readonly grid: boolean
	readonly dataEmpty: boolean
	readonly lastGpus: Record<string, GPUData>
}

export function GpuSummaryCharts({ chartData, grid, dataEmpty, lastGpus }: GpuSummaryChartsProps) {
	const summary = useMemo(() => buildGpuSummaryData(lastGpus), [lastGpus])
	const usageLegend = summary.usage.length > 1
	const vramLegend = summary.vram.length > 1

	return (
		<>
			<ChartCard
				legend={usageLegend}
				empty={dataEmpty}
				grid={grid}
				title={t`GPU Usage`}
				description={t`Utilization of all GPUs`}
			>
				<LineChartDefault
					legend={usageLegend}
					chartData={chartData}
					dataPoints={summary.usage}
					itemSorter={(left: { value: number }, right: { value: number }) => right.value - left.value}
					tickFormatter={(value) => `${toFixedFloat(value, 2)}%`}
					contentFormatter={({ value }) => `${decimalString(value)}%`}
				/>
			</ChartCard>

			{summary.vram.length > 0 && (
				<ChartCard
					legend={vramLegend}
					empty={dataEmpty}
					grid={grid}
					title={t`GPU VRAM`}
					description={t`Memory usage of all GPUs`}
				>
					<LineChartDefault
						legend={vramLegend}
						chartData={chartData}
						dataPoints={summary.vram}
						max={summary.vramMax}
						itemSorter={(left: { value: number }, right: { value: number }) => right.value - left.value}
						tickFormatter={(value) => {
							const formatted = formatBytes(value, false, Unit.Bytes, true)
							return `${toFixedFloat(formatted.value, formatted.value >= 10 ? 0 : 1)} ${formatted.unit}`
						}}
						contentFormatter={({ value }) => {
							const formatted = formatBytes(value, false, Unit.Bytes, true)
							return `${decimalString(formatted.value)} ${formatted.unit}`
						}}
					/>
				</ChartCard>
			)}
		</>
	)
}
