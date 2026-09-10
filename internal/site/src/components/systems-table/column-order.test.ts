import { describe, expect, test } from "bun:test"
import { getCoreRowModel, type ColumnDef, type ColumnSizingState, useReactTable } from "@tanstack/react-table"
import { createElement } from "react"
import { renderToStaticMarkup } from "react-dom/server"
import { ColumnDragHandle } from "./column-drag-handle"
import { moveColumn, orderCellsByColumnIds, pinColumnsToEnd } from "./column-order"
import { ReorderableTableHead } from "./reorderable-table-head"

type ProbeRow = {
	readonly system: string
	readonly cpu: number
	readonly memory: number
}

function ColumnOrderProbe() {
	const columns: ColumnDef<ProbeRow>[] = [
		{ accessorKey: "system", id: "system" },
		{ accessorKey: "cpu", id: "cpu" },
		{ accessorKey: "memory", id: "memory" },
	]
	const table = useReactTable({
		data: [],
		columns,
		getCoreRowModel: getCoreRowModel(),
		state: { columnOrder: ["memory", "cpu", "system"] },
	})
	return createElement(
		"output",
		null,
		table
			.getAllLeafColumns()
			.map((column) => column.id)
			.join(",")
	)
}

type HeaderDragProbeProps = {
	readonly columnSizing?: ColumnSizingState
}

function HeaderDragProbe({ columnSizing = {} }: HeaderDragProbeProps) {
	const table = useReactTable({
		data: [{ system: "alpha", cpu: 10, memory: 20 }],
		columns: [
			{
				accessorKey: "system",
				header: () => createElement(ColumnDragHandle, { label: "Drag column to reorder" }, "System"),
			},
			{
				accessorKey: "cpu",
				header: () => createElement(ColumnDragHandle, { label: "Drag column to reorder" }, "CPU"),
			},
			{
				accessorKey: "memory",
				header: () => createElement(ColumnDragHandle, { label: "Drag column to reorder" }, "Memory"),
			},
		],
		defaultColumn: {
			size: 100,
		},
		getCoreRowModel: getCoreRowModel(),
		state: { columnSizing },
	})

	return createElement(
		"table",
		null,
		createElement(ReorderableTableHead<ProbeRow>, {
			table,
		})
	)
}

describe("moveColumn", () => {
	test("moves a column to the target position", () => {
		// Given
		const columns = ["system", "cpu", "memory", "disk"] as const

		// When
		const reordered = moveColumn(columns, "cpu", "disk")

		// Then
		expect(reordered).toEqual(["system", "memory", "disk", "cpu"])
	})

	test("moves a column toward the start without dropping neighboring columns", () => {
		// Given
		const columns = ["system", "cpu", "memory", "disk"] as const

		// When
		const reordered = moveColumn(columns, "disk", "cpu")

		// Then
		expect(reordered).toEqual(["system", "disk", "cpu", "memory"])
	})

	test("keeps cell content and width attached to the column id after reordering", () => {
		// Given
		const cells = [
			{ column: { id: "system" }, content: "gpu-node-a", width: 220 },
			{ column: { id: "gpu" }, content: "92.5%", width: 140 },
			{ column: { id: "memory" }, content: "2 GB", width: 240 },
		] as const

		// When
		const reordered = orderCellsByColumnIds(cells, ["gpu", "memory", "system"])

		// Then
		expect(reordered).toEqual([
			{ column: { id: "gpu" }, content: "92.5%", width: 140 },
			{ column: { id: "memory" }, content: "2 GB", width: 240 },
			{ column: { id: "system" }, content: "gpu-node-a", width: 220 },
		])
	})

	test("selects only requested cells in the exact requested order", () => {
		// Given
		const cells = [
			{ column: { id: "system" }, content: "gpu-node-a" },
			{ column: { id: "cpu" }, content: "92.5%" },
			{ column: { id: "hidden" }, content: "not visible" },
		] as const

		// When
		const selected = orderCellsByColumnIds(cells, ["cpu", "missing", "system"])

		// Then
		expect(selected).toEqual([
			{ column: { id: "cpu" }, content: "92.5%" },
			{ column: { id: "system" }, content: "gpu-node-a" },
		])
	})

	test("keeps the order when either column is unavailable", () => {
		// Given
		const columns = ["system", "cpu", "memory"] as const

		// When / Then
		expect(moveColumn(columns, "missing", "cpu")).toEqual(["system", "cpu", "memory"])
		expect(moveColumn(columns, "cpu", "missing")).toEqual(["system", "cpu", "memory"])
	})

	test("reads leaf columns in the active TanStack column order", () => {
		// Given / When
		const markup = renderToStaticMarkup(createElement(ColumnOrderProbe))

		// Then
		expect(markup).toContain(">memory,cpu,system<")
	})

	test("renders a dedicated drag button without making the header cell natively draggable", () => {
		// Given
		const markup = renderToStaticMarkup(createElement(HeaderDragProbe, {}))

		// When / Then
		expect(markup.match(/<button(?=[^>]*data-column-drag-handle="true")(?=[^>]*type="button")[^>]*>/g)).toHaveLength(3)
		expect(markup).not.toContain('draggable="true"')
		expect(markup).not.toContain("<th draggable=")
	})

	test("keeps a fixed action column at the end while data columns move", () => {
		// Given
		const columns = ["system", "cpu", "memory", "actions"] as const

		// When
		const reordered = moveColumn(columns, "system", "memory", ["actions"])

		// Then
		expect(reordered).toEqual(["cpu", "memory", "system", "actions"])
	})

	test("does not move a fixed action column", () => {
		// Given
		const columns = ["system", "cpu", "memory", "actions"] as const

		// When / Then
		expect(moveColumn(columns, "actions", "system", ["actions"])).toEqual([...columns])
		expect(pinColumnsToEnd(["actions", "cpu", "system"], ["actions"])).toEqual(["cpu", "system", "actions"])
	})

	test("renders every default column at the same width", () => {
		// Given / When
		const markup = renderToStaticMarkup(createElement(HeaderDragProbe, {}))

		// Then
		expect(markup.match(/<col style="width:100px"\/>/g)).toHaveLength(3)
	})

	test("renders passive separators without resize controls", () => {
		// Given / When
		const markup = renderToStaticMarkup(createElement(HeaderDragProbe, {}))

		// Then
		expect(markup).not.toContain("data-column-resize-handle")
		expect(markup).not.toContain("cursor-col-resize")
		expect(markup.match(/after:bg-border/g)).toHaveLength(3)
	})

	test("applies an ID-bound automatic width only to its column", () => {
		// Given / When
		const markup = renderToStaticMarkup(createElement(HeaderDragProbe, { columnSizing: { cpu: 160 } }))

		// Then
		expect(markup.match(/<col style="width:100px"\/>/g)).toHaveLength(2)
		expect(markup.match(/<col style="width:160px"\/>/g)).toHaveLength(1)
	})
})
