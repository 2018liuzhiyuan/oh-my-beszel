type ColumnIdentified = {
	readonly column: {
		readonly id: string
	}
}

export function pinColumnsToEnd(columnIds: readonly string[], pinnedEndColumnIds: readonly string[]): string[] {
	const pinnedColumnIds = new Set(pinnedEndColumnIds)
	const movableColumns = columnIds.filter((columnId) => !pinnedColumnIds.has(columnId))
	const pinnedColumns = pinnedEndColumnIds.filter((columnId) => columnIds.includes(columnId))
	return [...movableColumns, ...pinnedColumns]
}

/** Keep fixed columns in place: select pinned first, actions pinned last. */
export function pinFixedColumns(
	columnIds: readonly string[],
	pinnedStartColumnIds: readonly string[],
	pinnedEndColumnIds: readonly string[]
): string[] {
	const startSet = new Set(pinnedStartColumnIds)
	const endSet = new Set(pinnedEndColumnIds)
	const movable = columnIds.filter((columnId) => !startSet.has(columnId) && !endSet.has(columnId))
	return [
		...pinnedStartColumnIds.filter((columnId) => columnIds.includes(columnId)),
		...movable,
		...pinnedEndColumnIds.filter((columnId) => columnIds.includes(columnId)),
	]
}

export function moveColumn(
	columnIds: readonly string[],
	activeId: string,
	targetId: string,
	pinnedEndColumnIds: readonly string[] = []
): string[] {
	const normalizedColumnIds = pinColumnsToEnd(columnIds, pinnedEndColumnIds)
	if (pinnedEndColumnIds.includes(activeId)) {
		return normalizedColumnIds
	}

	const activeIndex = normalizedColumnIds.indexOf(activeId)
	const targetIndex = normalizedColumnIds.indexOf(targetId)
	if (activeIndex < 0 || targetIndex < 0 || activeIndex === targetIndex) {
		return normalizedColumnIds
	}

	const reordered = [...normalizedColumnIds]
	const [activeColumn] = reordered.splice(activeIndex, 1)
	if (activeColumn === undefined) {
		return normalizedColumnIds
	}
	reordered.splice(targetIndex, 0, activeColumn)
	return pinColumnsToEnd(reordered, pinnedEndColumnIds)
}

export function orderCellsByColumnIds<T extends ColumnIdentified>(
	cells: readonly T[],
	columnIds: readonly string[]
): T[] {
	const cellsById = new Map(cells.map((cell) => [cell.column.id, cell]))
	return columnIds.flatMap((columnId) => {
		const cell = cellsById.get(columnId)
		return cell ? [cell] : []
	})
}
