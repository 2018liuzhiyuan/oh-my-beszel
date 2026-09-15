import type { Unit } from "@/lib/enums"
import { decimalString, formatBytes, formatTemperature, secondsToUptimeString } from "@/lib/utils"
import type { SystemRecord } from "@/types"
import { getContentAwareColumnWidth } from "./content-aware-column-width"

type SystemsTableColumnWidthRequest = {
	readonly id: string
	readonly header: string
	readonly hasSortControl: boolean
	readonly systems: readonly SystemRecord[]
	readonly unitNet?: Unit
	readonly unitTemp?: Unit
	readonly failedLabel: string
}

const FIXED_ACTIONS_WIDTH = 80
const FIXED_SELECT_WIDTH = 44
// the system cell hosts the row drag handle next to the name
const ROW_HANDLE_WIDTH = 36

function getColumnWidthLimits(id: string): { readonly min: number; readonly max: number } {
	// minimums stay low enough that the fixed select column plus every
	// visible column still fits a 1280px viewport (see distributeColumnWidths)
	switch (id) {
		case "system":
			return { min: 104, max: 320 }
		case "gpuFree":
			return { min: 112, max: 220 }
		case "net":
			return { min: 104, max: 176 }
		case "services":
			return { min: 128, max: 240 }
		case "uptime":
			return { min: 100, max: 192 }
		case "agent":
			return { min: 96, max: 160 }
		case "temperature":
			return { min: 100, max: 176 }
		default:
			return { min: 100, max: 176 }
	}
}

function formatNetworkRate(value: number, unitNet?: Unit): string {
	const formatted = formatBytes(value, true, unitNet, false)
	return `${decimalString(formatted.value, formatted.value >= 100 ? 1 : 2)} ${formatted.unit}`
}

function getColumnValues(request: SystemsTableColumnWidthRequest): readonly string[] {
	const { id, systems } = request
	switch (id) {
		case "system":
			return systems.map((system) => system.name)
		case "cpu":
			return systems.map((system) => `${system.info.cpu}%`)
		case "memory":
			return systems.map((system) => `${system.info.mp}%`)
		case "disk":
			return systems.map((system) => `${system.info.dp}%`)
		case "gpu":
			return systems.map((system) => `${system.info.g ?? 0}%`)
		case "vram":
			return systems.map((system) => `${system.info.gm ?? 0}%`)
		case "temperature":
			return systems.flatMap((system) => {
				const value = system.info.mt ?? system.info.dt
				if (value === undefined) return []
				const formatted = formatTemperature(value, request.unitTemp)
				return [`${Math.round(formatted.value)}${formatted.unit}`]
			})
		case "gpuFree":
			return systems.flatMap((system) => {
				const gpuId = system.info.gi
				return gpuId === undefined ? [] : [`${system.info.gf ?? 0} GB,GPU_${gpuId}`]
			})
		case "net":
			return systems.flatMap((system) =>
				system.info.bb === undefined ? [] : [formatNetworkRate(system.info.bb, request.unitNet)]
			)
		case "services":
			return systems.flatMap((system) => {
				const serviceCounts = system.info.sv
				return serviceCounts === undefined ? [] : [`${serviceCounts[0]} (${request.failedLabel}: ${serviceCounts[1]})`]
			})
		case "uptime":
			return systems.map((system) => secondsToUptimeString(system.info.u))
		case "agent":
			return systems.map((system) => system.info.v)
		default:
			return []
	}
}

/** Minimum width a column may shrink to when the table must fit the viewport. */
export function getSystemsTableColumnMinimum(id: string): number {
	return getColumnWidthLimits(id).min
}

export function getSystemsTableColumnWidth(request: SystemsTableColumnWidthRequest): number {
	if (request.id === "actions") return FIXED_ACTIONS_WIDTH
	if (request.id === "select") return FIXED_SELECT_WIDTH
	const limits = getColumnWidthLimits(request.id)
	return getContentAwareColumnWidth({
		header: request.header,
		values: getColumnValues(request),
		min: limits.min,
		max: limits.max,
		headerControls: request.hasSortControl ? 52 : 36,
		cellPadding: 24 + (request.id === "system" ? ROW_HANDLE_WIDTH : 0),
	})
}
