type ContentAwareColumnWidthRequest = {
	readonly header: string
	readonly values: readonly string[]
	readonly min: number
	readonly max: number
	readonly headerControls: number
	readonly cellPadding: number
}

const SPACING_GRID = 4
const ASCII_GLYPH_WIDTH = 7
const CJK_GLYPH_WIDTH = 14
const SPACE_GLYPH_WIDTH = 4

export function distributeColumnWidths(
	columnWidths: Readonly<Record<string, number>>,
	viewportWidth: number,
	fixedColumnIds: ReadonlySet<string>,
	minimumWidths: Readonly<Record<string, number>> = {}
): Record<string, number> {
	const distributed = { ...columnWidths }
	const entries = Object.entries(columnWidths)
	const intrinsicWidth = entries.reduce((total, [, width]) => total + width, 0)

	const flexibleEntries = entries.filter(([id]) => !fixedColumnIds.has(id))
	if (!flexibleEntries.length) return distributed

	if (viewportWidth < intrinsicWidth) {
		// shrink flexible columns proportionally, never below their minimum;
		// if the minimums still overflow, keep the overflow so the table scrolls
		shrinkColumns(distributed, flexibleEntries, viewportWidth, minimumWidths)
		return distributed
	}

	const fixedWidth = entries.reduce((total, [id, width]) => total + (fixedColumnIds.has(id) ? width : 0), 0)
	let remainingTargetWidth = viewportWidth - fixedWidth
	let remainingIntrinsicWidth = flexibleEntries.reduce((total, [, width]) => total + width, 0)

	for (const [index, [id, width]] of flexibleEntries.entries()) {
		const isLast = index === flexibleEntries.length - 1
		const nextWidth = isLast
			? remainingTargetWidth
			: Math.round((remainingTargetWidth * width) / remainingIntrinsicWidth / SPACING_GRID) * SPACING_GRID
		distributed[id] = nextWidth
		remainingTargetWidth -= nextWidth
		remainingIntrinsicWidth -= width
	}

	return distributed
}

function shrinkColumns(
	distributed: Record<string, number>,
	flexibleEntries: readonly (readonly [string, number])[],
	viewportWidth: number,
	minimumWidths: Readonly<Record<string, number>>
) {
	const flexibleIds = flexibleEntries.map(([id]) => id)
	const fixedTotal = Object.entries(distributed).reduce(
		(total, [id, width]) => total + (flexibleIds.includes(id) ? 0 : width),
		0
	)
	const target = viewportWidth - fixedTotal
	let flexibleTotal = flexibleEntries.reduce((total, [, width]) => total + width, 0)
	if (target >= flexibleTotal) return

	const original = { ...distributed }
	const atMinimum = new Set<string>()
	while (flexibleTotal > target) {
		const shrinkable = flexibleIds.filter((id) => !atMinimum.has(id))
		if (!shrinkable.length) {
			// minimums overflow the viewport: restore and let the table scroll
			Object.assign(distributed, original)
			return
		}
		const step = Math.min((flexibleTotal - target) / shrinkable.length, 1)
		for (const id of shrinkable) {
			const width = distributed[id] ?? 0
			const next = Math.max(minimumWidths[id] ?? 0, width - step)
			if (next === width) atMinimum.add(id)
			flexibleTotal -= width - next
			distributed[id] = next
		}
	}
}

function isWideGlyph(glyph: string): boolean {
	const codePoint = glyph.codePointAt(0)
	if (codePoint === undefined) return false
	return (
		(codePoint >= 0x2e80 && codePoint <= 0x9fff) ||
		(codePoint >= 0xac00 && codePoint <= 0xd7af) ||
		(codePoint >= 0xf900 && codePoint <= 0xfaff) ||
		(codePoint >= 0xff01 && codePoint <= 0xff60)
	)
}

function estimateTextWidth(value: string): number {
	return Array.from(value).reduce((width, glyph) => {
		if (/\s/u.test(glyph)) return width + SPACE_GLYPH_WIDTH
		return width + (isWideGlyph(glyph) ? CJK_GLYPH_WIDTH : ASCII_GLYPH_WIDTH)
	}, 0)
}

function roundUpToSpacingGrid(value: number): number {
	return Math.ceil(value / SPACING_GRID) * SPACING_GRID
}

export function getContentAwareColumnWidth(request: ContentAwareColumnWidthRequest): number {
	const headerWidth = estimateTextWidth(request.header) + request.headerControls
	const contentWidth = Math.max(0, ...request.values.map(estimateTextWidth)) + request.cellPadding
	return Math.min(request.max, Math.max(request.min, roundUpToSpacingGrid(Math.max(headerWidth, contentWidth))))
}
