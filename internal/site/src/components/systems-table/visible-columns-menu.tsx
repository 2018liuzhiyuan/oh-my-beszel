import { Trans, useLingui } from "@lingui/react/macro"
import { ArrowDownIcon, ArrowUpIcon, GripVerticalIcon } from "lucide-react"
import { useState } from "react"
import { Button } from "@/components/ui/button"
import { DropdownMenuCheckboxItem } from "@/components/ui/dropdown-menu"
import { cn } from "@/lib/utils"

export type ReorderableColumn = {
	readonly id: string
	readonly label: string
	readonly isVisible: boolean
	readonly isReorderable: boolean
	readonly toggleVisibility: (visible: boolean) => void
}

type VisibleColumnsMenuProps = {
	readonly columns: readonly ReorderableColumn[]
	readonly onMoveColumn: (activeId: string, targetId: string) => void
}

export function VisibleColumnsMenu({ columns, onMoveColumn }: VisibleColumnsMenuProps) {
	const { t } = useLingui()
	const [draggedColumnId, setDraggedColumnId] = useState<string | null>(null)
	const [dragOverColumnId, setDragOverColumnId] = useState<string | null>(null)
	const reorderableColumns = columns.filter((column) => column.isReorderable)

	const clearDragState = () => {
		setDraggedColumnId(null)
		setDragOverColumnId(null)
	}

	return (
		<div className="min-w-56 px-1.5 pb-1">
			<p className="px-2 pb-1 text-xs text-muted-foreground">
				<Trans>Drag or use arrows to reorder.</Trans>
			</p>
			{columns.map((column) => {
				const reorderableIndex = reorderableColumns.findIndex((reorderableColumn) => reorderableColumn.id === column.id)
				const previousColumn = reorderableColumns[reorderableIndex - 1]
				const nextColumn = reorderableColumns[reorderableIndex + 1]

				return (
					<fieldset
						key={column.id}
						aria-label={t`Reorder column`}
						draggable={column.isReorderable}
						data-drag-over={dragOverColumnId === column.id || undefined}
						className={cn(
							"group flex min-w-0 items-center rounded-sm border-0 p-0 data-[drag-over=true]:bg-accent/70",
							draggedColumnId === column.id && "opacity-50"
						)}
						onDragStart={(event) => {
							if (!column.isReorderable) return
							event.dataTransfer.effectAllowed = "move"
							event.dataTransfer.setData("text/plain", column.id)
							setDraggedColumnId(column.id)
						}}
						onDragEnter={() => {
							if (column.isReorderable && draggedColumnId && draggedColumnId !== column.id) {
								setDragOverColumnId(column.id)
							}
						}}
						onDragOver={(event) => {
							if (column.isReorderable && draggedColumnId && draggedColumnId !== column.id) {
								event.preventDefault()
								event.dataTransfer.dropEffect = "move"
							}
						}}
						onDrop={(event) => {
							event.preventDefault()
							if (column.isReorderable && draggedColumnId && draggedColumnId !== column.id) {
								onMoveColumn(draggedColumnId, column.id)
							}
							clearDragState()
						}}
						onDragEnd={clearDragState}
					>
						{column.isReorderable ? (
							<span
								className="ms-1 cursor-grab text-muted-foreground active:cursor-grabbing"
								title={t`Drag to reorder`}
							>
								<GripVerticalIcon className="size-4" aria-hidden="true" />
							</span>
						) : (
							<span className="ms-1 size-4" aria-hidden="true" />
						)}
						<DropdownMenuCheckboxItem
							onSelect={(event) => event.preventDefault()}
							checked={column.isVisible}
							onCheckedChange={(visible) => column.toggleVisibility(Boolean(visible))}
							className="min-w-0 flex-1 ps-7 pe-1"
						>
							<span className="truncate">{column.label}</span>
						</DropdownMenuCheckboxItem>
						{column.isReorderable && (
							<div className="flex shrink-0 pe-1">
								<Button
									type="button"
									variant="ghost"
									size="icon"
									className="size-7"
									aria-label={t`Move column up`}
									disabled={!previousColumn}
									onPointerDown={(event) => event.stopPropagation()}
									onClick={(event) => {
										event.preventDefault()
										event.stopPropagation()
										if (previousColumn) {
											onMoveColumn(column.id, previousColumn.id)
										}
									}}
								>
									<ArrowUpIcon className="size-3.5" aria-hidden="true" />
								</Button>
								<Button
									type="button"
									variant="ghost"
									size="icon"
									className="size-7"
									aria-label={t`Move column down`}
									disabled={!nextColumn}
									onPointerDown={(event) => event.stopPropagation()}
									onClick={(event) => {
										event.preventDefault()
										event.stopPropagation()
										if (nextColumn) {
											onMoveColumn(column.id, nextColumn.id)
										}
									}}
								>
									<ArrowDownIcon className="size-3.5" aria-hidden="true" />
								</Button>
							</div>
						)}
					</fieldset>
				)
			})}
		</div>
	)
}
