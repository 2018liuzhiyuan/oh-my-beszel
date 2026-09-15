import { useLingui } from "@lingui/react"
import { type Cell, type ColumnSizingState, flexRender, type Row } from "@tanstack/react-table"
import type { VirtualItem } from "@tanstack/react-virtual"
import { useSortable } from "@dnd-kit/sortable"
import { memo } from "react"
import { TableCell, TableRow } from "@/components/ui/table"
import { SystemStatus } from "@/lib/enums"
import { cn } from "@/lib/utils"
import type { SystemRecord } from "@/types"
import { orderCellsByColumnIds } from "./column-order"
import { RowDragHandleProvider } from "./row-dnd-provider"
import { useColumnDragOffset, useRowDrag } from "./table-dnd-provider"

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
	// actions and the checkbox pin to the row end, mirroring the header layout
	const draggableCells = orderedCells.filter((cell) => cell.column.id !== "select" && cell.column.id !== "actions")
	const endCells = orderedCells.filter((cell) => cell.column.id === "actions" || cell.column.id === "select")

	const { dragOffset, dropPosition } = useRowDrag(system.id)
	const { attributes, listeners, setActivatorNodeRef, setNodeRef } = useSortable({ id: system.id })

	return (
		<TableRow
			ref={setNodeRef}
			data-system-id={system.id}
			data-row-dragging={dragOffset !== null || undefined}
			data-row-drop-position={dropPosition}
			style={{
				transform: dragOffset === null ? undefined : `translate3d(0, ${dragOffset}px, 0)`,
			}}
			className={cn("cursor-pointer transition-opacity relative safari:transform-3d", {
				// drop target highlight; a tr::before line would consume a
				// table-fixed column slot in Chrome, shifting every cell
				"bg-accent/60": dropPosition !== undefined,
				"opacity-50": system.status === SystemStatus.Paused,
				"z-20 opacity-60": dragOffset !== null,
			})}
		>
			<RowDragHandleProvider bindings={{ attributes, listeners, setActivatorNodeRef }}>
				{draggableCells.map((cell) => (
					<DraggableSystemTableCell key={cell.id} cell={cell} columnSizing={columnSizing} rowHeight={virtualRow.size} />
				))}
				{endCells.map((cell) => (
					<FixedSystemTableCell key={cell.id} cell={cell} columnSizing={columnSizing} rowHeight={virtualRow.size} />
				))}
			</RowDragHandleProvider>
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

function FixedSystemTableCell({ cell, columnSizing, rowHeight }: SystemTableCellProps) {
	return (
		<TableCell
			data-column-id={cell.column.id}
			style={{ width: columnSizing[cell.column.id] ?? cell.column.getSize(), height: rowHeight }}
			className="py-0 px-0.5"
		>
			{flexRender(cell.column.columnDef.cell, cell.getContext())}
		</TableCell>
	)
}
