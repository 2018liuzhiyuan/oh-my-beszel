import { describe, expect, test } from "bun:test"
import { MANUAL_SORT_ID, mergeOrderedIds, moveRowOrderedIds, rowPositionMap } from "./row-order"

describe("rowPositionMap", () => {
	test("maps ids to their index", () => {
		expect(rowPositionMap(["a", "b", "c"]).get("b")).toBe(1)
	})

	test("unknown ids are absent", () => {
		expect(rowPositionMap(["a"]).has("z")).toBe(false)
	})

	test("manual is a stable sentinel", () => {
		expect(MANUAL_SORT_ID).toBe("manual")
	})
})

describe("moveRowOrderedIds", () => {
	test("moves a row down over a later row", () => {
		expect(moveRowOrderedIds(["a", "b", "c"], [], ["a", "b", "c"], "a", "c")).toEqual(["b", "c", "a"])
	})

	test("moves a row up over an earlier row", () => {
		expect(moveRowOrderedIds(["a", "b", "c"], [], ["a", "b", "c"], "c", "a")).toEqual(["c", "a", "b"])
	})

	test("no-op when active equals over", () => {
		expect(moveRowOrderedIds(["a", "b"], [], ["a", "b"], "a", "a")).toEqual(["a", "b"])
	})

	test("keeps stored positions of ids not currently visible", () => {
		const stored = ["a", "b", "hidden-x", "hidden-y"]
		const visible = ["b", "a"]
		// move a over b: visible becomes [a, b]; hidden ids keep their tail order
		expect(moveRowOrderedIds(visible, stored, ["a", "b", "hidden-x", "hidden-y"], "a", "b")).toEqual([
			"a",
			"b",
			"hidden-x",
			"hidden-y",
		])
	})

	test("appends untracked ids at the end", () => {
		expect(moveRowOrderedIds(["a", "b"], ["b"], ["a", "b", "new-1", "new-2"], "a", "b")).toEqual([
			"b",
			"a",
			"new-1",
			"new-2",
		])
	})

	test("unknown active or over ids leave the visible order unchanged", () => {
		expect(moveRowOrderedIds(["a", "b"], [], ["a", "b"], "zzz", "b")).toEqual(["a", "b"])
	})
})

describe("mergeOrderedIds", () => {
	test("deduplicates across all sources", () => {
		expect(mergeOrderedIds(["a", "b"], ["b", "c"], ["c", "d"])).toEqual(["a", "b", "c", "d"])
	})

	test("empty inputs yield empty order", () => {
		expect(mergeOrderedIds([], [], [])).toEqual([])
	})
})
