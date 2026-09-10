import {
	closestCenter,
	DndContext,
	type DragMoveEvent,
	type DragStartEvent,
	KeyboardSensor,
	MouseSensor,
	TouchSensor,
	type DragEndEvent,
	useSensor,
	useSensors,
} from "@dnd-kit/core"
import { restrictToHorizontalAxis, snapCenterToCursor } from "@dnd-kit/modifiers"
import { sortableKeyboardCoordinates } from "@dnd-kit/sortable"
import { createContext, type ReactNode, useContext, useState } from "react"

type ColumnDndProviderProps = {
	readonly children: ReactNode
	readonly onMoveColumn: (activeId: string, targetId: string) => void
}

type ColumnDragState = { readonly id: string | null; readonly offsetX: number }
const ColumnDragStateContext = createContext<ColumnDragState>({ id: null, offsetX: 0 })

export function useColumnDragOffset(columnId: string): number | null {
	const dragState = useContext(ColumnDragStateContext)
	return dragState.id === columnId ? dragState.offsetX : null
}

export function ColumnDndProvider({ children, onMoveColumn }: ColumnDndProviderProps) {
	const [dragState, setDragState] = useState<ColumnDragState>({ id: null, offsetX: 0 })
	const sensors = useSensors(
		useSensor(MouseSensor, { activationConstraint: { distance: 8 } }),
		useSensor(TouchSensor, { activationConstraint: { delay: 250, tolerance: 5 } }),
		useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates })
	)
	const handleDragStart = ({ active }: DragStartEvent) => setDragState({ id: String(active.id), offsetX: 0 })
	const handleDragMove = ({ active, delta }: DragMoveEvent) => setDragState({ id: String(active.id), offsetX: delta.x })

	const handleDragEnd = ({ active, over }: DragEndEvent) => {
		setDragState({ id: null, offsetX: 0 })
		if (over && active.id !== over.id) onMoveColumn(String(active.id), String(over.id))
	}

	return (
		<DndContext
			collisionDetection={closestCenter}
			modifiers={[snapCenterToCursor, restrictToHorizontalAxis]}
			sensors={sensors}
			onDragStart={handleDragStart}
			onDragMove={handleDragMove}
			onDragEnd={handleDragEnd}
			onDragCancel={() => setDragState({ id: null, offsetX: 0 })}
		>
			<ColumnDragStateContext.Provider value={dragState}>{children}</ColumnDragStateContext.Provider>
		</DndContext>
	)
}
