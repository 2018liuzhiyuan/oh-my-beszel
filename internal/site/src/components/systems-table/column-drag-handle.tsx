import { GripVerticalIcon } from "lucide-react"
import { createContext, type ReactNode, useContext } from "react"
import type { useSortable } from "@dnd-kit/sortable"

type ColumnDragBindings = Pick<ReturnType<typeof useSortable>, "attributes" | "listeners" | "setActivatorNodeRef">

const ColumnDragHandleContext = createContext<ColumnDragBindings | null>(null)

type ColumnDragHandleProviderProps = {
	readonly bindings: ColumnDragBindings
	readonly children: ReactNode
}

export function ColumnDragHandleProvider({ bindings, children }: ColumnDragHandleProviderProps) {
	return <ColumnDragHandleContext.Provider value={bindings}>{children}</ColumnDragHandleContext.Provider>
}

type ColumnDragHandleProps = {
	readonly label: string
}

export function ColumnDragHandle({ label }: ColumnDragHandleProps) {
	const bindings = useContext(ColumnDragHandleContext)

	return (
		<button
			type="button"
			ref={bindings?.setActivatorNodeRef}
			{...bindings?.attributes}
			{...bindings?.listeners}
			data-column-drag-handle
			aria-label={label}
			title={label}
			className="inline-flex size-7 shrink-0 touch-none items-center justify-center rounded-sm text-muted-foreground transition-colors hover:bg-accent hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring active:cursor-grabbing motion-reduce:transition-none"
		>
			<GripVerticalIcon className="size-4" aria-hidden="true" />
		</button>
	)
}
