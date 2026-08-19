/** biome-ignore-all lint/correctness/useHookAtTopLevel: Hooks live inside memoized column definitions */
import { t } from "@lingui/core/macro"
import { Trans, useLingui } from "@lingui/react/macro"
import { useStore } from "@nanostores/react"
import { getPagePath } from "@nanostores/router"
import type { CellContext, ColumnDef, HeaderContext } from "@tanstack/react-table"
import type { ClassValue } from "clsx"
import {
	ArrowUpDownIcon,
	ChevronRightSquareIcon,
	ClockArrowUp,
	CopyIcon,
	CpuIcon,
	HardDriveIcon,
	MemoryStickIcon,
	MoreHorizontalIcon,
	PauseCircleIcon,
	PenBoxIcon,
	PlayCircleIcon,
	ServerIcon,
	TerminalSquareIcon,
	Trash2Icon,
	WifiIcon,
} from "lucide-react"
import { memo, useMemo, useRef, useState } from "react"
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/tooltip"
import { isReadOnlyUser, pb } from "@/lib/api"
import { ConnectionType, connectionTypeLabels, MeterState, SystemStatus } from "@/lib/enums"
import { $longestSystemNameLen, $userSettings } from "@/lib/stores"
import { cn, copyToClipboard, decimalString, formatBytes, secondsToUptimeString } from "@/lib/utils"
import type { SystemRecord } from "@/types"
import { SystemDialog } from "../add-system"
import AlertButton from "../alerts/alert-button"
import { $router, Link } from "../router"
import {
	AlertDialog,
	AlertDialogAction,
	AlertDialogCancel,
	AlertDialogContent,
	AlertDialogDescription,
	AlertDialogFooter,
	AlertDialogHeader,
	AlertDialogTitle,
} from "../ui/alert-dialog"
import { Button, buttonVariants } from "../ui/button"
import { Dialog } from "../ui/dialog"
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "../ui/dropdown-menu"
import { EthernetIcon, GpuIcon, WebSocketIcon } from "../ui/icons"

const STATUS_COLORS = {
	[SystemStatus.Up]: "bg-green-500",
	[SystemStatus.Down]: "bg-red-500",
	[SystemStatus.Paused]: "bg-primary/40",
	[SystemStatus.Pending]: "bg-yellow-500",
} as const

function getMeterStateByThresholds(value: number, warn = 65, crit = 90): MeterState {
	return value >= crit ? MeterState.Crit : value >= warn ? MeterState.Warn : MeterState.Good
}

/**
 * @param viewMode - "table" or "grid"
 * @returns - Column definitions for the systems table
 */
export function SystemsTableColumns(viewMode: "table" | "grid"): ColumnDef<SystemRecord>[] {
	const columns = [
		{
			// size: 200,
			size: 100,
			minSize: 0,
			accessorKey: "name",
			id: "system",
			name: () => t`System`,
			sortingFn: (a, b) => a.original.name.localeCompare(b.original.name),
			filterFn: (() => {
				let filterInput = ""
				let filterInputLower = ""
				const nameCache = new Map<string, string>()
				const statusTranslations = {
					[SystemStatus.Up]: t`Up`.toLowerCase(),
					[SystemStatus.Down]: t`Down`.toLowerCase(),
					[SystemStatus.Paused]: t`Paused`.toLowerCase(),
				} as const

				// match filter value against name or translated status
				return (row, _, newFilterInput) => {
					const { name, status } = row.original
					if (newFilterInput !== filterInput) {
						filterInput = newFilterInput
						filterInputLower = newFilterInput.toLowerCase()
					}
					let nameLower = nameCache.get(name)
					if (nameLower === undefined) {
						nameLower = name.toLowerCase()
						nameCache.set(name, nameLower)
					}
					if (nameLower.includes(filterInputLower)) {
						return true
					}
					const statusLower = statusTranslations[status as keyof typeof statusTranslations]
					return statusLower?.includes(filterInputLower) || false
				}
			})(),
			enableHiding: false,
			invertSorting: false,
			Icon: ServerIcon,
			cell: (info) => {
				const { name, id } = info.row.original
				const longestName = useStore($longestSystemNameLen)
				const linkUrl = getPagePath($router, "system", { id })

				return (
					<>
						<span className="flex gap-2 items-center font-medium text-sm text-nowrap md:ps-1">
							<IndicatorDot system={info.row.original} />
							<Link
								href={linkUrl}
								tabIndex={-1}
								className="truncate z-10 relative"
								style={{ width: `${longestName / 1.05}ch` }}
								onMouseEnter={(e) => {
									// set title on hover if text is truncated to show full name
									const a = e.currentTarget
									if (a.scrollWidth > a.clientWidth) {
										a.title = name
									} else {
										a.removeAttribute("title")
									}
								}}
							>
								{name}
							</Link>
						</span>
						<Link href={linkUrl} className="inset-0 absolute size-full" aria-label={name}></Link>
					</>
				)
			},
			header: sortableHeader,
		},
		{
			accessorFn: ({ info }) => info.cpu,
			id: "cpu",
			name: () => t`CPU`,
			cell: TableCellWithMeter,
			Icon: CpuIcon,
			header: sortableHeader,
		},
		{
			// accessorKey: "info.mp",
			accessorFn: ({ info }) => info.mp,
			id: "memory",
			name: () => t`Memory`,
			cell: TableCellWithMeter,
			Icon: MemoryStickIcon,
			header: sortableHeader,
		},
		{
			accessorFn: ({ info }) => info.dp,
			id: "disk",
			name: () => t`Disk`,
			cell: (info: CellContext<SystemRecord, unknown>) =>
				info.row.original.info.efs ? DiskCellWithMultiple(info) : TableCellWithMeter(info),
			Icon: HardDriveIcon,
			header: sortableHeader,
		},
		{
			accessorFn: ({ info }) => info.g,
			id: "gpu",
			name: () => t({ message: "GPU_util", comment: "GPU utilization, systems table column" }),
			cell: TableCellWithMeter,
			Icon: GpuIcon,
			header: sortableHeader,
		},
		{
			accessorFn: ({ info }) => info.gm,
			id: "vram",
			name: () => t({ message: "VRAM", comment: "Video memory, systems table column" }),
			cell: TableCellWithMeter,
			Icon: MemoryStickIcon,
			header: sortableHeader,
		},
		{
			accessorFn: ({ info }) => (info.gi === undefined ? undefined : (info.gf ?? 0)),
			id: "gpuFree",
			name: () => t({ message: "VRAM Free", comment: "Free VRAM of the GPU with the most VRAM, systems table column" }),
			size: 50,
			Icon: MemoryStickIcon,
			header: sortableHeader,
			sortUndefined: "last",
			cell(info) {
				const { gi, gf } = info.row.original.info
				if (gi === undefined) {
					return null
				}
				const freeGb = gf ?? 0
				return (
					<span className={cn("tabular-nums whitespace-nowrap", freeGb === 0 && "text-red-500")}>
						{freeGb} GB,GPU_{gi}
					</span>
				)
			},
		},
		{
			accessorFn: ({ info, status }) => (status !== SystemStatus.Up ? undefined : info.bb),
			id: "net",
			name: () => t`Net`,
			size: 0,
			Icon: EthernetIcon,
			header: sortableHeader,
			sortUndefined: "last",
			cell(info) {
				const val = info.getValue() as number | undefined
				if (val === undefined) {
					return null
				}
				const userSettings = useStore($userSettings, { keys: ["unitNet"] })
				const { value, unit } = formatBytes(val, true, userSettings.unitNet, false)
				return (
					<span className="tabular-nums whitespace-nowrap">
						{decimalString(value, value >= 100 ? 1 : 2)} {unit}
					</span>
				)
			},
		},
		{
			accessorFn: ({ info }) => info.sv?.[0],
			id: "services",
			name: () => t`Services`,
			size: 50,
			Icon: TerminalSquareIcon,
			header: sortableHeader,
			hideSort: true,
			sortingFn: (a, b) => {
				// sort priorities: 1) failed services, 2) total services
				const [totalCountA, numFailedA] = a.original.info.sv ?? [0, 0]
				const [totalCountB, numFailedB] = b.original.info.sv ?? [0, 0]
				if (numFailedA !== numFailedB) {
					return numFailedA - numFailedB
				}
				return totalCountA - totalCountB
			},
			cell(info) {
				const sys = info.row.original
				const [totalCount, numFailed] = sys.info.sv ?? [0, 0]
				if (sys.status !== SystemStatus.Up || totalCount === 0) {
					return null
				}
				return (
					<span className="tabular-nums whitespace-nowrap flex gap-1.5 items-center">
						<span
							className={cn("block size-2 rounded-full", {
								[STATUS_COLORS[SystemStatus.Down]]: numFailed > 0,
								[STATUS_COLORS[SystemStatus.Up]]: numFailed === 0,
							})}
						/>
						{totalCount}{" "}
						<span className="text-muted-foreground text-sm -ms-0.5">
							({t`Failed`.toLowerCase()}: {numFailed})
						</span>
					</span>
				)
			},
		},
		{
			accessorFn: ({ info }) => info.u || undefined,
			id: "uptime",
			name: () => t`Uptime`,
			size: 50,
			Icon: ClockArrowUp,
			header: sortableHeader,
			hideSort: true,
			cell(info) {
				const uptime = info.getValue() as number
				if (!uptime) {
					return null
				}
				return <span className="tabular-nums whitespace-nowrap">{secondsToUptimeString(uptime)}</span>
			},
		},
		{
			accessorFn: ({ info }) => info.v,
			id: "agent",
			name: () => t`Agent`,
			size: 50,
			Icon: WifiIcon,
			hideSort: true,
			header: sortableHeader,
			cell(info) {
				const version = info.getValue() as string
				if (!version) {
					return null
				}
				const system = info.row.original
				const color = {
					"text-green-500": version === globalThis.BESZEL.HUB_VERSION,
					"text-yellow-500": version !== globalThis.BESZEL.HUB_VERSION,
					"text-red-500": system.status !== SystemStatus.Up,
				}
				return (
					<Link
						href={getPagePath($router, "system", { id: system.id })}
						className={cn(
							"flex gap-1.5 items-center md:pe-5 tabular-nums relative z-10",
							viewMode === "table" && "ps-0.5"
						)}
						tabIndex={-1}
						title={connectionTypeLabels[system.info.ct as ConnectionType]}
						role="none"
					>
						{system.info.ct === ConnectionType.WebSocket && (
							<WebSocketIcon className={cn("size-3 pointer-events-none", color)} />
						)}
						{system.info.ct === ConnectionType.SSH && (
							<ChevronRightSquareIcon className={cn("size-3 pointer-events-none", color)} />
						)}
						{!system.info.ct && <IndicatorDot system={system} className={cn(color, "bg-current mx-0.5")} />}
						<span className="truncate max-w-14">{info.getValue() as string}</span>
					</Link>
				)
			},
		},
		{
			id: "actions",
			// @ts-expect-error
			name: () => t({ message: "Actions", comment: "Table column" }),
			size: 50,
			cell: ({ row }) => (
				<div className="relative z-10 flex justify-end items-center gap-1 -ms-3">
					<AlertButton system={row.original} />
					<ActionsButton system={row.original} />
				</div>
			),
		},
	] as ColumnDef<SystemRecord>[]

	const preferredOrder = [
		"system",
		"gpu",
		"vram",
		"gpuFree",
		"cpu",
		"memory",
		"net",
		"disk",
		"uptime",
		"services",
		"agent",
		"actions",
	]
	const orderIndex = new Map(preferredOrder.map((id, index) => [id, index]))
	return columns.sort((a, b) => {
		const aIndex = orderIndex.get(a.id ?? "") ?? preferredOrder.length
		const bIndex = orderIndex.get(b.id ?? "") ?? preferredOrder.length
		return aIndex - bIndex
	})
}

function sortableHeader(context: HeaderContext<SystemRecord, unknown>) {
	const { column } = context
	// @ts-expect-error
	const { Icon, hideSort, name }: { Icon: React.ElementType; name: () => string; hideSort: boolean } = column.columnDef
	const isSorted = column.getIsSorted()
	return (
		<Button
			variant="ghost"
			className={cn("h-9 px-3 flex duration-50", isSorted && "bg-accent/70 light:bg-accent text-accent-foreground/90")}
			onClick={() => column.toggleSorting(column.getIsSorted() === "asc")}
		>
			{Icon && <Icon className="me-2 size-4" />}
			{name()}
			{hideSort || <ArrowUpDownIcon className="ms-2 size-4" />}
		</Button>
	)
}

function TableCellWithMeter(info: CellContext<SystemRecord, unknown>) {
	const { colorWarn = 65, colorCrit = 90 } = useStore($userSettings, { keys: ["colorWarn", "colorCrit"] })
	const rawValue = info.getValue()
	if (rawValue === undefined || rawValue === null) {
		return null
	}
	const val = Number(rawValue) || 0
	const threshold = getMeterStateByThresholds(val, colorWarn, colorCrit)
	const meterClass = cn(
		"h-full",
		(info.row.original.status !== SystemStatus.Up && STATUS_COLORS.paused) ||
			(threshold === MeterState.Good && STATUS_COLORS.up) ||
			(threshold === MeterState.Warn && STATUS_COLORS.pending) ||
			STATUS_COLORS.down
	)
	return (
		<div className="flex gap-2 items-center tabular-nums tracking-tight w-full">
			<span className="min-w-8 shrink-0">{decimalString(val, val >= 10 ? 1 : 2)}%</span>
			<span className="flex-1 min-w-8 grid bg-muted h-[1em] rounded-sm overflow-hidden">
				<span className={meterClass} style={{ width: `${val}%` }}></span>
			</span>
		</div>
	)
}

function DiskCellWithMultiple(info: CellContext<SystemRecord, unknown>) {
	const { colorWarn = 65, colorCrit = 90 } = useStore($userSettings, { keys: ["colorWarn", "colorCrit"] })
	const { info: sysInfo, status, id } = info.row.original
	const extraFs = Object.entries(sysInfo.efs ?? {})
	const rootDiskPct = sysInfo.dp

	// sort extra disks by percentage descending
	extraFs.sort((a, b) => b[1] - a[1])

	function getIndicatorColor(pct: number) {
		const threshold = getMeterStateByThresholds(pct, colorWarn, colorCrit)
		return (
			(status !== SystemStatus.Up && STATUS_COLORS.paused) ||
			(threshold === MeterState.Good && STATUS_COLORS.up) ||
			(threshold === MeterState.Warn && STATUS_COLORS.pending) ||
			STATUS_COLORS.down
		)
	}

	function getMeterClass(pct: number) {
		return cn("h-full", getIndicatorColor(pct))
	}

	// Extra disk indicators (max 3 dots - one per state if any disk exists in range)
	const stateColors = [STATUS_COLORS.up, STATUS_COLORS.pending, STATUS_COLORS.down]
	const extraDiskIndicators =
		status !== SystemStatus.Up
			? []
			: [...new Set(extraFs.map(([, pct]) => getMeterStateByThresholds(pct, colorWarn, colorCrit)))]
					.sort()
					.map((state) => stateColors[state])

	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<Link
					href={getPagePath($router, "system", { id })}
					tabIndex={-1}
					className="flex flex-col gap-0.5 w-full relative z-10"
				>
					<div className="flex gap-2 items-center tabular-nums tracking-tight">
						<span className="min-w-8 shrink-0">{decimalString(rootDiskPct, rootDiskPct >= 10 ? 1 : 2)}%</span>
						<span className="flex-1 min-w-8 flex items-center gap-0.5 px-1 justify-end bg-muted h-[1em] rounded-sm overflow-hidden relative">
							{/* Root disk */}
							<span
								className={cn("absolute inset-0", getMeterClass(rootDiskPct))}
								style={{ width: `${rootDiskPct}%` }}
							></span>
							{/* Extra disk indicators */}
							{extraDiskIndicators.map((color) => (
								<span
									key={color}
									className={cn("size-1.5 rounded-full shrink-0 outline-[0.5px] outline-muted", color)}
								/>
							))}
						</span>
					</div>
				</Link>
			</TooltipTrigger>
			<TooltipContent side="right" className="max-w-xs pb-2">
				<div className="grid gap-1">
					<div className="grid gap-0.5">
						<div className="text-[0.65rem] text-muted-foreground uppercase tracking-wide tabular-nums">
							<Trans context="Root disk label">Root</Trans>
						</div>
						<div className="flex gap-2 items-center tabular-nums text-xs">
							<span className="min-w-7">{decimalString(rootDiskPct, rootDiskPct >= 10 ? 1 : 2)}%</span>
							<span className="flex-1 min-w-12 grid bg-muted h-2.5 rounded-sm overflow-hidden">
								<span className={getMeterClass(rootDiskPct)} style={{ width: `${rootDiskPct}%` }}></span>
							</span>
						</div>
					</div>
					{extraFs.map(([name, pct]) => {
						return (
							<div key={name} className="grid gap-0.5">
								<div className="text-[0.65rem] max-w-40 text-muted-foreground uppercase tracking-wide truncate">
									{name}
								</div>
								<div className="flex gap-2 items-center tabular-nums text-xs">
									<span className="min-w-7">{decimalString(pct, pct >= 10 ? 1 : 2)}%</span>
									<span className="flex-1 min-w-12 grid bg-muted h-2.5 rounded-sm overflow-hidden">
										<span className={getMeterClass(pct)} style={{ width: `${pct}%` }}></span>
									</span>
								</div>
							</div>
						)
					})}
				</div>
			</TooltipContent>
		</Tooltip>
	)
}

export function IndicatorDot({ system, className }: { system: SystemRecord; className?: ClassValue }) {
	className ||= STATUS_COLORS[system.status as keyof typeof STATUS_COLORS] || ""
	return (
		<span
			className={cn("shrink-0 size-2 rounded-full", className)}
			// style={{ marginBottom: "-1px" }}
		/>
	)
}

export const ActionsButton = memo(({ system }: { system: SystemRecord }) => {
	const [deleteOpen, setDeleteOpen] = useState(false)
	const [editOpen, setEditOpen] = useState(false)
	const editOpened = useRef(false)
	const { t } = useLingui()
	const { id, status, host, name } = system

	return useMemo(() => {
		return (
			<>
				<DropdownMenu>
					<DropdownMenuTrigger asChild>
						<Button variant="ghost" size={"icon"}>
							<span className="sr-only">
								<Trans>Open menu</Trans>
							</span>
							<MoreHorizontalIcon className="w-5" />
						</Button>
					</DropdownMenuTrigger>
					<DropdownMenuContent align="end">
						{!isReadOnlyUser() && (
							<DropdownMenuItem
								onSelect={() => {
									editOpened.current = true
									setEditOpen(true)
								}}
							>
								<PenBoxIcon className="me-2.5 size-4" />
								<Trans>Edit</Trans>
							</DropdownMenuItem>
						)}
						<DropdownMenuItem
							className={cn(isReadOnlyUser() && "hidden")}
							onClick={() => {
								pb.collection("systems").update(id, {
									status: status === SystemStatus.Paused ? SystemStatus.Pending : SystemStatus.Paused,
								})
							}}
						>
							{status === SystemStatus.Paused ? (
								<>
									<PlayCircleIcon className="me-2.5 size-4" />
									<Trans>Resume</Trans>
								</>
							) : (
								<>
									<PauseCircleIcon className="me-2.5 size-4" />
									<Trans>Pause</Trans>
								</>
							)}
						</DropdownMenuItem>
						<DropdownMenuItem onClick={() => copyToClipboard(name)}>
							<CopyIcon className="me-2.5 size-4" />
							<Trans>Copy name</Trans>
						</DropdownMenuItem>
						<DropdownMenuItem onClick={() => copyToClipboard(host)}>
							<CopyIcon className="me-2.5 size-4" />
							<Trans>Copy host</Trans>
						</DropdownMenuItem>
						<DropdownMenuSeparator className={cn(isReadOnlyUser() && "hidden")} />
						<DropdownMenuItem className={cn(isReadOnlyUser() && "hidden")} onSelect={() => setDeleteOpen(true)}>
							<Trash2Icon className="me-2.5 size-4" />
							<Trans>Delete</Trans>
						</DropdownMenuItem>
					</DropdownMenuContent>
				</DropdownMenu>
				{/* edit dialog */}
				<Dialog open={editOpen} onOpenChange={setEditOpen}>
					{editOpened.current && <SystemDialog system={system} setOpen={setEditOpen} />}
				</Dialog>
				{/* deletion dialog */}
				<AlertDialog open={deleteOpen} onOpenChange={(open) => setDeleteOpen(open)}>
					<AlertDialogContent>
						<AlertDialogHeader>
							<AlertDialogTitle>
								<Trans>Are you sure you want to delete {name}?</Trans>
							</AlertDialogTitle>
							<AlertDialogDescription>
								<Trans>
									This action cannot be undone. This will permanently delete all current records for {name} from the
									database.
								</Trans>
							</AlertDialogDescription>
						</AlertDialogHeader>
						<AlertDialogFooter>
							<AlertDialogCancel>
								<Trans>Cancel</Trans>
							</AlertDialogCancel>
							<AlertDialogAction
								className={cn(buttonVariants({ variant: "destructive" }))}
								onClick={() => pb.collection("systems").delete(id)}
							>
								<Trans>Continue</Trans>
							</AlertDialogAction>
						</AlertDialogFooter>
					</AlertDialogContent>
				</AlertDialog>
			</>
		)
	}, [id, status, host, name, system, t, deleteOpen, editOpen])
})
