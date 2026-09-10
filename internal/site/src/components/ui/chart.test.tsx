import { expect, test } from "bun:test"
import { sortTooltipItems } from "./sort-tooltip-items"

test("sorts tooltip items when the comparator uses both items", () => {
	// Given
	const payload = [
		{ name: "Low", value: 1 },
		{ name: "High", value: 2 },
	]

	// When
	const sorted = sortTooltipItems(payload, (left, right) => right.value - left.value)

	// Then
	expect(sorted.map((item) => item.name)).toEqual(["High", "Low"])
	expect(payload.map((item) => item.name)).toEqual(["Low", "High"])
})
