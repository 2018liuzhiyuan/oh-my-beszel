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

function getColumnWidthLimits(id: string): { readonly min: number; readonly max: number } {
	switch (id) {
		case "system":
			return { min: 104, max: 320 }
		case "gpuFree":
			return { min: 120, max: 220 }
		case "net":
			return { min: 112, max: 176 }
		case "services":
			return { min: 136, max: 240 }
		case "uptime":
			return { min: 104, max: 192 }
		case "agent":
			return { min: 96, max: 160 }
		case "temperature":
			return { min: 108, max: 176 }
		default:
			return { min: 108, max: 176 }
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

export function getSystemsTableColumnWidth(request: SystemsTableColumnWidthRequest): number {
	if (request.id === "actions") return FIXED_ACTIONS_WIDTH
	const limits = getColumnWidthLimits(request.id)
	return getContentAwareColumnWidth({
		header: request.header,
		values: getColumnValues(request),
		min: limits.min,
		max: limits.max,
		headerControls: request.hasSortControl ? 52 : 36,
		cellPadding: 24,
	})
}
