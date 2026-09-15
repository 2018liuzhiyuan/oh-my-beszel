import {
	closestCenter,
	DndContext,
	type DragEndEvent,
	type DragMoveEvent,
	type DragOverEvent,
	type DragStartEvent,
	KeyboardSensor,
	type Modifier,
	MouseSensor,
	TouchSensor,
	useSensor,
	useSensors,
} from "@dnd-kit/core"
import { restrictToHorizontalAxis, restrictToVerticalAxis, snapCenterToCursor } from "@dnd-kit/modifiers"
import { sortableKeyboardCoordinates, SortableContext, verticalListSortingStrategy } from "@dnd-kit/sortable"
import { createContext, type ReactNode, useContext, useMemo, useState } from "react"

/**
 * One DndContext for both header-column and body-row reordering.
 *
 * A nested second DndContext is not an option: dnd-kit renders helper divs
 * after its children, so a context placed inside <table> injects invalid
 * DOM that consumes table-fixed column slots, while a context wrapping the
 * whole table steals the header sortables from the column context.
 */

type TableDndProviderProps = {
	readonly children: ReactNode
	readonly orderedColumnIds: readonly string[]
	readonly orderedRowIds: readonly string[]
	readonly onMoveColumn: (activeId: string, targetId: string) => void
	readonly onMoveRow: (activeId: string, overId: string) => void
}

type DragState = {
	readonly activeId: string | null
	readonly activeIsRow: boolean
	readonly activeIndex: number
	readonly overId: string | null
	readonly overIndex: number
	readonly offsetX: number
	readonly offsetY: number
}

const initialDragState: DragState = {
	activeId: null,
	activeIsRow: false,
	activeIndex: -1,
	overId: null,
	overIndex: -1,
	offsetX: 0,
	offsetY: 0,
}
const DragStateContext = createContext<DragState>(initialDragState)

/** Horizontal preview offset for a dragged column's body cells. */
export function useColumnDragOffset(columnId: string): number | null {
	const dragState = useContext(DragStateContext)
	return !dragState.activeIsRow && dragState.activeId === columnId ? dragState.offsetX : null
}

export type RowDropPosition = "above" | "below"

/** Vertical drag feedback for one table row: live offset for the dragged row, drop edge for the target row. */
export function useRowDrag(rowId: string): {
	readonly dragOffset: number | null
	readonly dropPosition: RowDropPosition | undefined
	readonly isActive: boolean
} {
	const dragState = useContext(DragStateContext)
	const isActive = dragState.activeIsRow && dragState.activeId === rowId
	const dropPosition =
		dragState.overId === rowId &&
		dragState.activeId !== null &&
		dragState.activeId !== rowId &&
		dragState.activeIndex >= 0
			? dragState.activeIndex < dragState.overIndex
				? "below"
				: "above"
			: undefined
	return { dragOffset: isActive ? dragState.offsetY : null, dropPosition, isActive }
}

export function TableDndProvider({
	children,
	orderedColumnIds,
	orderedRowIds,
	onMoveColumn,
	onMoveRow,
}: TableDndProviderProps) {
	const [dragState, setDragState] = useState<DragState>(initialDragState)
	const rowIdSet = useMemo(() => new Set(orderedRowIds), [orderedRowIds])

	// column drags move horizontally, row drags vertically
	const restrictToActiveAxis: Modifier = (args) => {
		const isRow = args.active ? rowIdSet.has(String(args.active.id)) : false
		return isRow ? restrictToVerticalAxis(args) : restrictToHorizontalAxis(args)
	}

	const sensors = useSensors(
		useSensor(MouseSensor, { activationConstraint: { distance: 8 } }),
		useSensor(TouchSensor, { activationConstraint: { delay: 250, tolerance: 5 } }),
		useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates })
	)

	const indexIn = (id: string, isRow: boolean) => (isRow ? orderedRowIds : orderedColumnIds).indexOf(id)

	const handleDragStart = ({ active }: DragStartEvent) => {
		const activeId = String(active.id)
		const activeIsRow = rowIdSet.has(activeId)
		setDragState({ ...initialDragState, activeId, activeIsRow, activeIndex: indexIn(activeId, activeIsRow) })
	}
	const handleDragOver = ({ over }: DragOverEvent) => {
		const overId = over ? String(over.id) : null
		setDragState((state) => {
			if (state.activeId === null || overId === null) return state
			const overIndex = indexIn(overId, state.activeIsRow)
			if (overIndex < 0 || (state.overId === overId && state.overIndex === overIndex)) return state
			return { ...state, overId, overIndex }
		})
	}
	const handleDragMove = ({ active, delta }: DragMoveEvent) => {
		setDragState((state) => {
			if (state.activeId === null || String(active.id) !== state.activeId) return state
			return state.activeIsRow ? { ...state, offsetY: delta.y } : { ...state, offsetX: delta.x }
		})
	}
	const handleDragEnd = ({ active, over }: DragEndEvent) => {
		const activeId = String(active.id)
		const activeIsRow = rowIdSet.has(activeId)
		const overId = over ? String(over.id) : null
		setDragState(initialDragState)
		if (!overId || activeId === overId) return
		if (activeIsRow) {
			if (rowIdSet.has(overId)) onMoveRow(activeId, overId)
		} else if (orderedColumnIds.includes(overId)) {
			onMoveColumn(activeId, overId)
		}
	}

	return (
		<DndContext
			collisionDetection={closestCenter}
			modifiers={[snapCenterToCursor, restrictToActiveAxis]}
			sensors={sensors}
			onDragStart={handleDragStart}
			onDragOver={handleDragOver}
			onDragMove={handleDragMove}
			onDragEnd={handleDragEnd}
			onDragCancel={() => setDragState(initialDragState)}
		>
			{/* the header renders its own SortableContext for columns; this one serves the rows */}
			<SortableContext items={orderedRowIds} strategy={verticalListSortingStrategy}>
				<DragStateContext.Provider value={dragState}>{children}</DragStateContext.Provider>
			</SortableContext>
		</DndContext>
	)
}
