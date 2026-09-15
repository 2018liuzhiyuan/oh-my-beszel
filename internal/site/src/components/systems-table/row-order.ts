/** Manual row ordering for the systems table, persisted client-side. */

export const MANUAL_SORT_ID = "manual"

/** Position lookup for manually ordered system ids; ids absent from the list sort last. */
export function rowPositionMap(orderedIds: readonly string[]): Map<string, number> {
	return new Map(orderedIds.map((id, index) => [id, index]))
}

/**
 * Apply an active->over row move within the visible id sequence, then merge with
 * the stored order so ids not currently visible keep their relative positions
 * and untracked ids land at the end.
 */
export function moveRowOrderedIds(
	visibleIds: readonly string[],
	storedOrder: readonly string[],
	allIds: readonly string[],
	activeId: string,
	overId: string
): string[] {
	const visible = [...visibleIds]
	const activeIndex = visible.indexOf(activeId)
	const overIndex = visible.indexOf(overId)
	if (activeIndex >= 0 && overIndex >= 0 && activeIndex !== overIndex) {
		const [active] = visible.splice(activeIndex, 1)
		if (active !== undefined) visible.splice(overIndex, 0, active)
	}
	return mergeOrderedIds(visible, storedOrder, allIds)
}

/** Visible order first, then stored ids not visible, then any remaining ids. */
export function mergeOrderedIds(
	visibleIds: readonly string[],
	storedOrder: readonly string[],
	allIds: readonly string[]
): string[] {
	const seen = new Set(visibleIds)
	const merged = [...visibleIds]
	for (const id of storedOrder) {
		if (!seen.has(id)) {
			seen.add(id)
			merged.push(id)
		}
	}
	for (const id of allIds) {
		if (!seen.has(id)) {
			seen.add(id)
			merged.push(id)
		}
	}
	return merged
}
