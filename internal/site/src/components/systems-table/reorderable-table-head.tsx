import { useDndContext } from "@dnd-kit/core"
import { horizontalListSortingStrategy, SortableContext, useSortable } from "@dnd-kit/sortable"
import { CSS } from "@dnd-kit/utilities"
import { type ColumnSizingState, flexRender, type Header, type Table as TableType } from "@tanstack/react-table"
import { ColumnDragHandleProvider } from "./column-drag-handle"

type ReorderableTableHeadProps<TData> = {
	readonly table: TableType<TData>
	readonly columnSizing?: ColumnSizingState
}

type DropPosition = "before" | "after"

// columns that never participate in header drag reordering
const FIXED_COLUMN_IDS = new Set(["actions", "select"])

export function ReorderableTableHead<TData>({ table, columnSizing = {} }: ReorderableTableHeadProps<TData>) {
	const draggableColumnIds = table
		.getVisibleLeafColumns()
		.filter((column) => !FIXED_COLUMN_IDS.has(column.id))
		.map((column) => column.id)

	return (
		<>
			<colgroup>
				{table.getVisibleLeafColumns().map((column) => (
					<col key={column.id} style={{ width: columnSizing[column.id] ?? column.getSize() }} />
				))}
			</colgroup>
			<thead className="sticky top-0 z-50 w-full border-b-2 bg-table-header border-border/50 [&_tr]:border-b">
				{table.getHeaderGroups().map((headerGroup) => (
					<SortableContext key={headerGroup.id} items={draggableColumnIds} strategy={horizontalListSortingStrategy}>
						<tr>
							{headerGroup.headers.map((header) =>
								FIXED_COLUMN_IDS.has(header.column.id) ? (
									<FixedHeaderCell key={header.id} header={header} />
								) : (
									<DraggableHeaderCell
										key={header.id}
										header={header}
										width={columnSizing[header.column.id] ?? header.getSize()}
										draggableColumnIds={draggableColumnIds}
									/>
								)
							)}
						</tr>
					</SortableContext>
				))}
			</thead>
		</>
	)
}

type DraggableHeaderCellProps<TData> = {
	readonly header: Header<TData, unknown>
	readonly width: number
	readonly draggableColumnIds: readonly string[]
}

function DraggableHeaderCell<TData>({ header, width, draggableColumnIds }: DraggableHeaderCellProps<TData>) {
	const columnId = header.column.id
	const { active, over } = useDndContext()
	const { attributes, isDragging, listeners, setActivatorNodeRef, setNodeRef, transform, transition } = useSortable({
		id: columnId,
		transition: { duration: 200, easing: "cubic-bezier(0.2, 0, 0, 1)" },
	})
	const activeId = active ? String(active.id) : null
	const overId = over ? String(over.id) : null
	const activeIndex = activeId ? draggableColumnIds.indexOf(activeId) : -1
	const columnIndex = draggableColumnIds.indexOf(columnId)
	const dropPosition: DropPosition | undefined =
		overId === columnId && activeId !== columnId && activeIndex >= 0
			? activeIndex < columnIndex
				? "after"
				: "before"
			: undefined
	const horizontalTransform = transform ? { ...transform, y: 0 } : null

	return (
		<th
			ref={setNodeRef}
			data-column-dnd-item
			data-column-id={columnId}
			data-dragging={isDragging || undefined}
			data-drop-position={dropPosition}
			style={{
				width,
				transform: CSS.Transform.toString(horizontalTransform),
				transition,
			}}
			className={[
				"relative h-12 p-0 text-start align-middle font-medium text-muted-foreground select-none",
				"after:pointer-events-none after:absolute after:end-0 after:top-1/2 after:h-6 after:w-px after:-translate-y-1/2 after:bg-border",
				"transition-[transform,opacity] duration-200 ease-[cubic-bezier(0.2,0,0,1)] motion-reduce:transition-none",
				"before:pointer-events-none before:absolute before:inset-y-0 before:z-30 before:hidden before:w-0.5 before:bg-primary before:content-['']",
				"data-[drop-position=before]:before:start-0 data-[drop-position=before]:before:block",
				"data-[drop-position=after]:before:end-0 data-[drop-position=after]:before:block",
				isDragging ? "z-30 opacity-60 shadow-sm" : "",
			]
				.filter(Boolean)
				.join(" ")}
		>
			<ColumnDragHandleProvider bindings={{ attributes, listeners, setActivatorNodeRef }}>
				{flexRender(header.column.columnDef.header, header.getContext())}
			</ColumnDragHandleProvider>
		</th>
	)
}

function FixedHeaderCell<TData>({ header }: { readonly header: Header<TData, unknown> }) {
	return (
		<th
			data-column-id={header.column.id}
			style={{ width: header.getSize() }}
			className="relative h-12 p-0 text-start align-middle font-medium text-muted-foreground"
		>
			{flexRender(header.column.columnDef.header, header.getContext())}
		</th>
	)
}
