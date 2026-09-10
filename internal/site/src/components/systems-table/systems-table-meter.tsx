import { useStore } from "@nanostores/react"
import { MeterState, SystemStatus } from "@/lib/enums"
import { $userSettings } from "@/lib/stores"
import { cn, decimalString } from "@/lib/utils"
import type { SystemRecord } from "@/types"
import { MeterBar } from "./systems-table-meter-bar"

export const STATUS_COLORS = {
	[SystemStatus.Up]: "bg-green-500",
	[SystemStatus.Down]: "bg-red-500",
	[SystemStatus.Paused]: "bg-primary/40",
	[SystemStatus.Pending]: "bg-yellow-500",
} as const

export function getMeterStateByThresholds(value: number, warn = 65, crit = 90): MeterState {
	return value >= crit ? MeterState.Crit : value >= warn ? MeterState.Warn : MeterState.Good
}

type MeterCellContext = {
	getValue: () => unknown
	row: {
		original: Pick<SystemRecord, "status">
	}
}

export function TableCellWithMeter(info: MeterCellContext) {
	const { colorWarn = 65, colorCrit = 90 } = useStore($userSettings, { keys: ["colorWarn", "colorCrit"] })
	const rawValue = info.getValue()
	if (rawValue === undefined || rawValue === null) {
		return null
	}
	const val = Number(rawValue) || 0
	const threshold = getMeterStateByThresholds(val, colorWarn, colorCrit)
	const meterClass = cn(
		(info.row.original.status !== SystemStatus.Up && STATUS_COLORS.paused) ||
			(threshold === MeterState.Good && STATUS_COLORS.up) ||
			(threshold === MeterState.Warn && STATUS_COLORS.pending) ||
			STATUS_COLORS.down
	)
	return (
		<div className="flex gap-2 items-center tabular-nums tracking-tight w-full min-w-0">
			<span className="min-w-8 shrink-0">{decimalString(val, val >= 10 ? 1 : 2)}%</span>
			<MeterBar value={val} fillClassName={meterClass} />
		</div>
	)
}
