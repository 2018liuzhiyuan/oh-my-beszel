import { useLingui } from "@lingui/react"
import { type Cell, type ColumnSizingState, flexRender, type Row } from "@tanstack/react-table"
import type { VirtualItem } from "@tanstack/react-virtual"
import { memo } from "react"
import { TableCell, TableRow } from "@/components/ui/table"
import { SystemStatus } from "@/lib/enums"
import { cn } from "@/lib/utils"
import type { SystemRecord } from "@/types"
import { orderCellsByColumnIds } from "./column-order"
import { useColumnDragOffset } from "./column-dnd-provider"

type SystemTableRowProps = {
	readonly row: Row<SystemRecord>
	readonly virtualRow: VirtualItem
	readonly columnIds: readonly string[]
	readonly columnSizing: ColumnSizingState
}

export const SystemTableRow = memo(({ row, virtualRow, columnIds, columnSizing }: SystemTableRowProps) => {
	const system = row.original
	useLingui()
	const orderedCells = orderCellsByColumnIds(row.getAllCells(), columnIds)
	const draggableCells = orderedCells.filter((cell) => cell.column.id !== "actions")
	const actionsCell = orderedCells.find((cell) => cell.column.id === "actions")

	return (
		<TableRow
			data-system-id={system.id}
			className={cn("cursor-pointer transition-opacity relative safari:transform-3d", {
				"opacity-50": system.status === SystemStatus.Paused,
			})}
		>
			{draggableCells.map((cell) => (
				<DraggableSystemTableCell key={cell.id} cell={cell} columnSizing={columnSizing} rowHeight={virtualRow.size} />
			))}
			{actionsCell && (
				<FixedSystemTableCell cell={actionsCell} columnSizing={columnSizing} rowHeight={virtualRow.size} />
			)}
		</TableRow>
	)
})

type SystemTableCellProps = {
	readonly cell: Cell<SystemRecord, unknown>
	readonly columnSizing: ColumnSizingState
	readonly rowHeight: number
}

function DraggableSystemTableCell({ cell, columnSizing, rowHeight }: SystemTableCellProps) {
	const dragOffset = useColumnDragOffset(cell.column.id)

	return (
		<TableCell
			data-column-dnd-item
			data-column-id={cell.column.id}
			data-dragging={dragOffset !== null || undefined}
			style={{
				width: columnSizing[cell.column.id] ?? cell.column.getSize(),
				height: rowHeight,
				transform: dragOffset === null ? undefined : `translate3d(${dragOffset}px, 0, 0)`,
			}}
			className={cn(
				"relative px-3 py-0 transition-[transform,opacity] duration-200 ease-[cubic-bezier(0.2,0,0,1)] motion-reduce:transition-none data-[dragging=true]:z-20 data-[dragging=true]:opacity-60"
			)}
		>
			{flexRender(cell.column.columnDef.cell, cell.getContext())}
		</TableCell>
	)
}

function FixedSystemTableCell({ cell, rowHeight }: SystemTableCellProps) {
	return (
		<TableCell
			data-column-id={cell.column.id}
			style={{ width: cell.column.getSize(), height: rowHeight }}
			className="py-0 px-0.5"
		>
			{flexRender(cell.column.columnDef.cell, cell.getContext())}
		</TableCell>
	)
}
