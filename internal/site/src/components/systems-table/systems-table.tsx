import { getSystemPath } from "@/lib/system-links"
import { Trans, useLingui } from "@lingui/react/macro"
import { useStore } from "@nanostores/react"
import {
	type Column,
	type ColumnDef,
	type ColumnFiltersState,
	type ColumnOrderState,
	type ColumnSizingState,
	flexRender,
	getCoreRowModel,
	getFilteredRowModel,
	getSortedRowModel,
	type Row,
	type RowSelectionState,
	type SortingState,
	type Table as TableType,
	useReactTable,
	type VisibilityState,
} from "@tanstack/react-table"
import { useVirtualizer } from "@tanstack/react-virtual"
import {
	ArrowDownIcon,
	ArrowUpDownIcon,
	ArrowUpIcon,
	EyeIcon,
	FilterIcon,
	LayoutGridIcon,
	LayoutListIcon,
	Settings2Icon,
	XIcon,
} from "lucide-react"
import { Fragment, memo, useCallback, useEffect, useMemo, useRef, useState } from "react"
import { Button } from "@/components/ui/button"
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuLabel,
	DropdownMenuRadioGroup,
	DropdownMenuRadioItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Input } from "@/components/ui/input"
import { TableBody, TableCell, TableRow } from "@/components/ui/table"
import { SystemStatus } from "@/lib/enums"
import { isReadOnlyUser } from "@/lib/api"
import { $downSystems, $pausedSystems, $systems, $upSystems, $userSettings } from "@/lib/stores"
import { cn, runOnce, useBrowserStorage } from "@/lib/utils"
import type { SystemRecord } from "@/types"
import AlertButton from "../alerts/alert-button"
import { Link } from "../router"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "../ui/card"
import { SystemsTableColumns, ActionsButton, IndicatorDot } from "./systems-table-columns"
import { moveColumn, pinFixedColumns } from "./column-order"
import { TableDndProvider } from "./table-dnd-provider"
import { distributeColumnWidths } from "./content-aware-column-width"
import { MANUAL_SORT_ID, mergeOrderedIds, moveRowOrderedIds, rowPositionMap } from "./row-order"
import { BulkActionsBar } from "./bulk-actions-bar"
import { ReorderableTableHead } from "./reorderable-table-head"
import { SystemTableRow } from "./system-table-row"
import { getSystemsTableColumnMinimum, getSystemsTableColumnWidth } from "./systems-table-column-width"
import { VisibleColumnsMenu } from "./visible-columns-menu"

type ViewMode = "table" | "grid"
type StatusFilter = "all" | SystemRecord["status"]
type NamedColumnDef = { readonly name: () => string; readonly hideSort?: boolean }
const FIXED_WIDTH_COLUMN_IDS = new Set(["actions", "select"])
const MANUAL_SORTING: SortingState = [{ id: MANUAL_SORT_ID, desc: false }]

function hasColumnName(columnDef: object): columnDef is NamedColumnDef {
	return "name" in columnDef && typeof columnDef.name === "function"
}

function getColumnName(column: Column<SystemRecord>): string {
	return hasColumnName(column.columnDef) ? column.columnDef.name() : column.id
}

const preloadSystemDetail = runOnce(() => import("@/components/routes/system.tsx"))

export default function SystemsTable() {
	const data = useStore($systems)
	const downSystems = $downSystems.get()
	const upSystems = $upSystems.get()
	const pausedSystems = $pausedSystems.get()
	const { i18n, t } = useLingui()
	const { unitNet, unitTemp } = useStore($userSettings, { keys: ["unitNet", "unitTemp"] })
	const [filter, setFilter] = useState<string>("")
	const [statusFilter, setStatusFilter] = useState<StatusFilter>("all")
	const [sorting, setSorting] = useBrowserStorage<SortingState>(
		"sortMode",
		[{ id: "system", desc: false }],
		sessionStorage
	)
	const [columnFilters, setColumnFilters] = useState<ColumnFiltersState>([])
	const [columnVisibility, setColumnVisibility] = useBrowserStorage<VisibilityState>("gpuDashboardColsV1", {
		services: false,
		agent: false,
	})
	const [columnOrder, setColumnOrder] = useBrowserStorage<ColumnOrderState>("systemsTableColumnOrderV1", [])
	const [rowOrder, setRowOrder] = useBrowserStorage<string[]>("systemsTableRowOrderV1", [])
	const [rowSelection, setRowSelection] = useState<RowSelectionState>({})
	const manualSortActive = sorting[0]?.id === MANUAL_SORT_ID

	const locale = i18n.locale

	// Filter data based on status filter
	const filteredData = useMemo(() => {
		let systems = data
		if (statusFilter === SystemStatus.Up) {
			systems = Object.values(upSystems) ?? []
		} else if (statusFilter === SystemStatus.Down) {
			systems = Object.values(downSystems) ?? []
		} else if (statusFilter === SystemStatus.Paused) {
			systems = Object.values(pausedSystems) ?? []
		}
		if (!manualSortActive) return systems
		// manual row order: known positions first, then untracked systems by name
		const positions = rowPositionMap(rowOrder)
		return [...systems].sort((a, b) => {
			const positionA = positions.get(a.id) ?? Number.MAX_SAFE_INTEGER
			const positionB = positions.get(b.id) ?? Number.MAX_SAFE_INTEGER
			return positionA !== positionB ? positionA - positionB : a.name.localeCompare(b.name)
		})
	}, [data, statusFilter, upSystems, downSystems, pausedSystems, manualSortActive, rowOrder])

	const [viewMode, setViewMode] = useBrowserStorage<ViewMode>(
		"viewMode",
		// show grid view on mobile if there are less than 200 systems (looks better but table is more efficient)
		window.innerWidth < 1024 && filteredData.length < 200 ? "grid" : "table"
	)
	const canSelectRows = viewMode === "table" && !isReadOnlyUser()

	useEffect(() => {
		if (filter !== undefined) {
			table.getColumn("system")?.setFilterValue(filter)
		}
	}, [filter])

	const columnDefs = useMemo(
		() =>
			SystemsTableColumns(viewMode)
				// the checkbox column only exists for the desktop table view
				.filter((column) => column.id !== "select" || (viewMode === "table" && !isReadOnlyUser()))
				.map((column) => {
					const id = column.id ?? ""
					const namedColumn = hasColumnName(column)
					const size = getSystemsTableColumnWidth({
						id,
						header: namedColumn ? column.name() : id,
						hasSortControl: namedColumn && !column.hideSort,
						systems: filteredData,
						unitNet,
						unitTemp,
						failedLabel: t`Failed`.toLowerCase(),
					})
					return { ...column, minSize: size, size, maxSize: size, enableResizing: false }
				}),
		[filteredData, locale, t, unitNet, unitTemp, viewMode]
	)

	const normalizedColumnOrder = useMemo(() => {
		// materialize the full order: TanStack appends unlisted columns after
		// listed ones, so the far-right checkbox column must be listed last
		const movable = columnDefs
			.map((column) => column.id ?? "")
			.filter((id: string) => id !== "actions" && id !== "select")
		const storedMovable = columnOrder.filter((id: string) => movable.includes(id))
		const orderedMovable = mergeOrderedIds(storedMovable, movable, movable)
		const endIds = canSelectRows ? ["actions", "select"] : ["actions"]
		return [...orderedMovable, ...endIds.filter((id) => columnDefs.some((column) => column.id === id))]
	}, [columnDefs, columnOrder, canSelectRows])

	const table = useReactTable({
		data: filteredData,
		columns: columnDefs,
		getCoreRowModel: getCoreRowModel(),
		getRowId: (row) => row.id,
		onSortingChange: setSorting,
		getSortedRowModel: getSortedRowModel(),
		onColumnFiltersChange: setColumnFilters,
		getFilteredRowModel: getFilteredRowModel(),
		onColumnVisibilityChange: setColumnVisibility,
		onColumnOrderChange: setColumnOrder,
		onRowSelectionChange: setRowSelection,
		enableRowSelection: viewMode === "table" && !isReadOnlyUser(),
		enableColumnResizing: false,
		state: {
			sorting,
			columnFilters,
			columnVisibility,
			columnOrder: normalizedColumnOrder,
			rowSelection,
		},
		defaultColumn: {
			invertSorting: true,
			sortUndefined: "last",
			enableResizing: false,
		},
	})
	const columnSizing = useMemo<ColumnSizingState>(
		() => Object.fromEntries(table.getAllLeafColumns().map((column) => [column.id, column.getSize()])),
		[table, columnDefs]
	)

	const rows = table.getRowModel().rows
	const columns = table.getAllLeafColumns()
	const visibleColumns = table.getVisibleLeafColumns()
	const selectedSystems = useMemo(() => rows.filter((row) => row.getIsSelected()).map((row) => row.original), [rows])
	const visibleColumnIds = useMemo(
		() => table.getVisibleLeafColumns().map((column) => column.id),
		[table, normalizedColumnOrder, columnVisibility, viewMode]
	)

	const handleMoveRow = useCallback(
		(activeId: string, overId: string) => {
			const visibleIds = rows.map((row) => row.original.id)
			setRowOrder(
				moveRowOrderedIds(
					visibleIds,
					rowOrder,
					data.map(({ id }) => id),
					activeId,
					overId
				)
			)
			if (!manualSortActive) setSorting(MANUAL_SORTING)
		},
		[rows, rowOrder, data, manualSortActive, setRowOrder, setSorting]
	)

	const [upSystemsLength, downSystemsLength, pausedSystemsLength] = useMemo(() => {
		return [Object.values(upSystems).length, Object.values(downSystems).length, Object.values(pausedSystems).length]
	}, [upSystems, downSystems, pausedSystems])

	const CardHead = useMemo(() => {
		return (
			<CardHeader className="p-0 mb-3 sm:mb-4">
				<div className="grid md:flex gap-x-5 gap-y-3 w-full items-end">
					<div className="px-2 sm:px-1">
						<CardTitle className="mb-2">
							<Trans>All Systems</Trans>
						</CardTitle>
						<CardDescription className="flex">
							<Trans>Click on a system to view more information.</Trans>
						</CardDescription>
					</div>

					<div className="flex gap-2 ms-auto w-full md:w-80">
						<div className="relative flex-1">
							<Input
								placeholder={t`Filter...`}
								onChange={(e) => setFilter(e.target.value)}
								value={filter}
								className="ps-4 pe-10 w-full"
							/>
							{filter && (
								<Button
									type="button"
									variant="ghost"
									size="icon"
									aria-label={t`Clear`}
									className="absolute right-1 top-1/2 -translate-y-1/2 h-7 w-7 text-muted-foreground"
									onClick={() => setFilter("")}
								>
									<XIcon className="h-4 w-4" />
								</Button>
							)}
						</div>
						<DropdownMenu>
							<DropdownMenuTrigger asChild>
								<Button variant="outline">
									<Settings2Icon className="me-1.5 size-4 opacity-80" />
									<Trans>View</Trans>
								</Button>
							</DropdownMenuTrigger>
							<DropdownMenuContent align="end" className="h-72 md:h-auto min-w-48 md:min-w-auto overflow-y-auto">
								<div className="grid grid-cols-1 md:grid-cols-4 divide-y md:divide-s md:divide-y-0">
									<div className="border-r">
										<DropdownMenuLabel className="pt-2 px-3.5 flex items-center gap-2">
											<LayoutGridIcon className="size-4" />
											<Trans>Layout</Trans>
										</DropdownMenuLabel>
										<DropdownMenuSeparator />
										<DropdownMenuRadioGroup
											className="px-1 pb-1"
											value={viewMode}
											onValueChange={(view) => setViewMode(view as ViewMode)}
										>
											<DropdownMenuRadioItem value="table" onSelect={(e) => e.preventDefault()} className="gap-2">
												<LayoutListIcon className="size-4" />
												<Trans>Table</Trans>
											</DropdownMenuRadioItem>
											<DropdownMenuRadioItem value="grid" onSelect={(e) => e.preventDefault()} className="gap-2">
												<LayoutGridIcon className="size-4" />
												<Trans>Grid</Trans>
											</DropdownMenuRadioItem>
										</DropdownMenuRadioGroup>
									</div>

									<div className="border-r">
										<DropdownMenuLabel className="pt-2 px-3.5 flex items-center gap-2">
											<FilterIcon className="size-4" />
											<Trans>Status</Trans>
										</DropdownMenuLabel>
										<DropdownMenuSeparator />
										<DropdownMenuRadioGroup
											className="px-1 pb-1"
											value={statusFilter}
											onValueChange={(value) => setStatusFilter(value as StatusFilter)}
										>
											<DropdownMenuRadioItem value="all" onSelect={(e) => e.preventDefault()}>
												<Trans>All Systems</Trans>
											</DropdownMenuRadioItem>
											<DropdownMenuRadioItem value="up" onSelect={(e) => e.preventDefault()}>
												<Trans>Up ({upSystemsLength})</Trans>
											</DropdownMenuRadioItem>
											<DropdownMenuRadioItem value="down" onSelect={(e) => e.preventDefault()}>
												<Trans>Down ({downSystemsLength})</Trans>
											</DropdownMenuRadioItem>
											<DropdownMenuRadioItem value="paused" onSelect={(e) => e.preventDefault()}>
												<Trans>Paused ({pausedSystemsLength})</Trans>
											</DropdownMenuRadioItem>
										</DropdownMenuRadioGroup>
									</div>

									<div className="border-r">
										<DropdownMenuLabel className="pt-2 px-3.5 flex items-center gap-2">
											<ArrowUpDownIcon className="size-4" />
											<Trans>Sort By</Trans>
										</DropdownMenuLabel>
										<DropdownMenuSeparator />
										<div className="px-1 pb-1">
											{columns.map((column) => {
												if (!column.getCanSort()) return null
												let Icon = <span className="w-6"></span>
												// if current sort column, show sort direction
												if (sorting[0]?.id === column.id) {
													if (sorting[0]?.desc) {
														Icon = <ArrowUpIcon className="me-2 size-4" />
													} else {
														Icon = <ArrowDownIcon className="me-2 size-4" />
													}
												}
												return (
													<DropdownMenuItem
														onSelect={(e) => {
															e.preventDefault()
															setSorting([{ id: column.id, desc: sorting[0]?.id === column.id && !sorting[0]?.desc }])
														}}
														key={column.id}
													>
														{Icon}
														{getColumnName(column)}
													</DropdownMenuItem>
												)
											})}
										</div>
									</div>

									<div>
										<DropdownMenuLabel className="pt-2 px-3.5 flex items-center gap-2">
											<EyeIcon className="size-4" />
											<Trans>Visible Fields</Trans>
										</DropdownMenuLabel>
										<DropdownMenuSeparator />
										<VisibleColumnsMenu
											columns={columns
												.filter((column) => column.getCanHide())
												.map((column) => ({
													id: column.id,
													label: getColumnName(column),
													isVisible: column.getIsVisible(),
													isReorderable: column.id !== "actions",
													toggleVisibility: (visible) => column.toggleVisibility(visible),
												}))}
											onMoveColumn={(activeId, targetId) => {
												setColumnOrder(
													pinFixedColumns(
														moveColumn(
															table.getAllLeafColumns().map((column) => column.id),
															activeId,
															targetId,
															["actions"]
														),
														[],
														["actions", "select"]
													)
												)
											}}
										/>
									</div>
								</div>
							</DropdownMenuContent>
						</DropdownMenu>
					</div>
				</div>
			</CardHeader>
		)
	}, [
		visibleColumns.length,
		sorting,
		viewMode,
		locale,
		statusFilter,
		upSystemsLength,
		downSystemsLength,
		pausedSystemsLength,
		filter,
		normalizedColumnOrder,
	])

	return (
		<Card className="w-full px-3 py-5 sm:py-6 sm:px-6">
			{CardHead}
			{selectedSystems.length > 0 && (
				<BulkActionsBar
					systems={selectedSystems}
					onFinished={(failedIds) => setRowSelection(Object.fromEntries(failedIds.map((id) => [id, true])))}
				/>
			)}
			{viewMode === "table" ? (
				// table layout
				<div className="rounded-md">
					<AllSystemsTable
						table={table}
						rows={rows}
						columnIds={visibleColumnIds}
						columnSizing={columnSizing}
						onMoveRow={handleMoveRow}
					/>
				</div>
			) : (
				// grid layout
				<div className="grid gap-4 grid-cols-1 sm:grid-cols-2 lg:grid-cols-3">
					{rows?.length ? (
						rows.map((row) => {
							return (
								<SystemCard
									key={row.original.id}
									row={row}
									table={table}
									columnIds={visibleColumnIds}
									locale={locale}
								/>
							)
						})
					) : (
						<div className="col-span-full text-center py-8">
							<Trans>No systems found.</Trans>
						</div>
					)}
				</div>
			)}
		</Card>
	)
}

const AllSystemsTable = memo(
	({
		table,
		rows,
		columnIds,
		columnSizing,
		onMoveRow,
	}: {
		readonly table: TableType<SystemRecord>
		readonly rows: Row<SystemRecord>[]
		readonly columnIds: readonly string[]
		readonly columnSizing: ColumnSizingState
		readonly onMoveRow: (activeId: string, overId: string) => void
	}) => {
		// The virtualizer will need a reference to the scrollable container element
		const scrollRef = useRef<HTMLDivElement>(null)
		const [viewportWidth, setViewportWidth] = useState(0)

		useEffect(() => {
			const scrollElement = scrollRef.current
			if (!scrollElement) return
			const updateViewportWidth = () => setViewportWidth(scrollElement.clientWidth)
			updateViewportWidth()
			const resizeObserver = new ResizeObserver(updateViewportWidth)
			resizeObserver.observe(scrollElement)
			return () => resizeObserver.disconnect()
		}, [])

		const virtualizer = useVirtualizer<HTMLDivElement, HTMLTableRowElement>({
			count: rows.length,
			estimateSize: () => (rows.length > 10 ? 56 : 60),
			getScrollElement: () => scrollRef.current,
			overscan: 5,
		})
		const virtualRows = virtualizer.getVirtualItems()

		const paddingTop = Math.max(0, virtualRows[0]?.start ?? 0 - virtualizer.options.scrollMargin)
		const paddingBottom = Math.max(0, virtualizer.getTotalSize() - (virtualRows[virtualRows.length - 1]?.end ?? 0))
		const visibleColumnSizing = useMemo(
			() => Object.fromEntries(columnIds.map((id) => [id, columnSizing[id] ?? table.getColumn(id)?.getSize() ?? 0])),
			[columnIds, columnSizing, table]
		)
		const minimumColumnWidths = useMemo(
			() => Object.fromEntries(columnIds.map((id) => [id, getSystemsTableColumnMinimum(id)])),
			[columnIds]
		)
		const renderedColumnSizing = useMemo(
			() => distributeColumnWidths(visibleColumnSizing, viewportWidth, FIXED_WIDTH_COLUMN_IDS, minimumColumnWidths),
			[viewportWidth, visibleColumnSizing, minimumColumnWidths]
		)
		// base the table width on the DISTRIBUTED widths so a shrunk layout does not overflow
		const renderedTableWidth = Math.max(
			Object.values(renderedColumnSizing).reduce((total, width) => total + width, 0),
			viewportWidth
		)
		const orderedRowIds = useMemo(() => rows.map((row) => row.original.id), [rows])

		return (
			<div
				className={cn(
					"h-min max-h-[calc(100dvh-17rem)] w-full max-w-full relative overflow-auto border rounded-md",
					// don't set min height if there are less than 2 rows, do set if we need to display the empty state
					(!rows.length || rows.length > 2) && "min-h-50"
				)}
				ref={scrollRef}
			>
				{/* add header height to table size */}
				<div style={{ height: `${virtualizer.getTotalSize() + 50}px`, paddingTop, paddingBottom }}>
					{/* single DndContext outside the table: its helper divs must never
					    land inside <table>, where they would consume column slots */}
					<TableDndProvider
						orderedColumnIds={columnIds}
						orderedRowIds={orderedRowIds}
						onMoveColumn={(activeId, targetId) => {
							table.setColumnOrder(
								pinFixedColumns(
									moveColumn(
										table.getAllLeafColumns().map((column) => column.id),
										activeId,
										targetId,
										["actions"]
									),
									[],
									["actions", "select"]
								)
							)
						}}
						onMoveRow={onMoveRow}
					>
						<table className="table-fixed text-sm h-full" style={{ width: renderedTableWidth }}>
							{/* remount on order change so the head can never render a stale header order */}
							<SystemsTableHead key={columnIds.join(",")} table={table} columnSizing={renderedColumnSizing} />
							<TableBody onPointerEnter={preloadSystemDetail}>
								{rows.length ? (
									virtualRows.map((virtualRow) => {
										const row = rows[virtualRow.index] as Row<SystemRecord>
										return (
											<SystemTableRow
												key={row.id}
												row={row}
												virtualRow={virtualRow}
												columnIds={columnIds}
												columnSizing={renderedColumnSizing}
											/>
										)
									})
								) : (
									<TableRow>
										<TableCell colSpan={columnIds.length} className="h-37 text-center pointer-events-none">
											<Trans>No systems found.</Trans>
										</TableCell>
									</TableRow>
								)}
							</TableBody>
						</table>
					</TableDndProvider>
				</div>
			</div>
		)
	}
)

function SystemsTableHead({
	table,
	columnSizing,
}: {
	table: TableType<SystemRecord>
	columnSizing: ColumnSizingState
}) {
	return <ReorderableTableHead table={table} columnSizing={columnSizing} />
}

const SystemCard = memo(
	({
		row,
		table,
		columnIds,
		locale,
	}: {
		row: Row<SystemRecord>
		table: TableType<SystemRecord>
		columnIds: readonly string[]
		locale: string
	}) => {
		const system = row.original
		const cellsById = new Map(row.getAllCells().map((cell) => [cell.column.id, cell]))

		return (
			<Card
				onPointerEnter={preloadSystemDetail}
				key={`${system.id}-${locale}`}
				className={cn(
					"cursor-pointer hover:shadow-md transition-all bg-transparent w-full dark:border-border duration-200 relative",
					{
						"opacity-50": system.status === SystemStatus.Paused,
					}
				)}
			>
				<CardHeader className="py-1 ps-4 pe-2 bg-muted/30 border-b border-border/60">
					<div className="flex items-center gap-1 w-full overflow-hidden">
						<h3 className="text-primary/90 min-w-0 flex-1 gap-2.5 font-semibold">
							<div className="flex items-center gap-2.5 min-w-0 flex-1">
								<IndicatorDot system={system} />
								<span className="text-[.95em]/normal tracking-normal text-primary/90 truncate">{system.name}</span>
							</div>
						</h3>
						{table.getColumn("actions")?.getIsVisible() && (
							<div className="flex gap-1 shrink-0 relative z-10">
								<AlertButton system={system} />
								<ActionsButton system={system} />
							</div>
						)}
					</div>
				</CardHeader>
				<CardContent className="text-sm px-5 pt-3.5 pb-4">
					<div
						className="grid gap-2.5"
						style={{ gridTemplateColumns: "24px minmax(80px, max-content) minmax(0, 1fr)" }}
					>
						{columnIds.map((columnId) => {
							if (columnId === "system" || columnId === "actions" || columnId === "select") return null
							const column = table.getColumn(columnId)
							if (!column) return null
							const cell = cellsById.get(column.id)
							if (!cell) return null
							// @ts-expect-error
							const { Icon, name } = column.columnDef as ColumnDef<SystemRecord, unknown>
							return (
								<Fragment key={column.id}>
									<div className="flex items-center">
										{column.id === "lastSeen" ? (
											<EyeIcon className="size-4 text-muted-foreground" />
										) : (
											Icon && <Icon className="size-4 text-muted-foreground" />
										)}
									</div>
									<div className="flex items-center text-muted-foreground pr-3">{name()}:</div>
									<div className="flex items-center min-w-0">
										{flexRender(cell.column.columnDef.cell, cell.getContext())}
									</div>
								</Fragment>
							)
						})}
					</div>
				</CardContent>
				<Link
					data-system-id={row.original.id}
					href={getSystemPath(row.original.id)}
					className="inset-0 absolute w-full h-full"
				>
					<span className="sr-only">{row.original.name}</span>
				</Link>
			</Card>
		)
	}
)
