export type TooltipItemComparator<T> = (left: T, right: T) => number

export function sortTooltipItems<T>(items: readonly T[], comparator: TooltipItemComparator<T>): T[] {
	return [...items].sort(comparator)
}
