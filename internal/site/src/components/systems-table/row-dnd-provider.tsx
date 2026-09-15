import type { useSortable } from "@dnd-kit/sortable"
import { GripVerticalIcon } from "lucide-react"
import { createContext, type ReactNode, useContext } from "react"
import { cn } from "@/lib/utils"

type RowSortableBindings = Pick<ReturnType<typeof useSortable>, "attributes" | "listeners" | "setActivatorNodeRef">

const RowDragHandleContext = createContext<RowSortableBindings | null>(null)

/** Provides a row's sortable bindings to the drag handle rendered deep in its cells. */
export function RowDragHandleProvider({
	bindings,
	children,
}: {
	readonly bindings: RowSortableBindings
	readonly children: ReactNode
}) {
	return <RowDragHandleContext.Provider value={bindings}>{children}</RowDragHandleContext.Provider>
}

/** Vertical drag handle for a system row; only meaningful inside the table's row SortableContext. */
export function RowDragHandle({ label }: { readonly label: string }) {
	const bindings = useContext(RowDragHandleContext)
	return (
		<button
			type="button"
			ref={bindings?.setActivatorNodeRef}
			{...bindings?.attributes}
			{...bindings?.listeners}
			data-row-drag-handle
			aria-label={label}
			title={label}
			className={cn(
				"relative z-20 -ms-1 me-0 inline-flex size-7 shrink-0 touch-none items-center justify-center rounded-sm text-muted-foreground/50 transition-colors hover:bg-accent hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring active:cursor-grabbing"
			)}
		>
			<GripVerticalIcon className="size-4" aria-hidden="true" />
		</button>
	)
}
