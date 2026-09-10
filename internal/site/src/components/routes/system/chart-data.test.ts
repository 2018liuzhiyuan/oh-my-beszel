import { describe, expect, test } from "bun:test"
import { appendData } from "./append-data"

type TestRecord = {
	created: string | number | null
	stats: { cpu: number } | null
}

describe("appendData", () => {
	test("keeps sparse valid samples connected", () => {
		const previous: TestRecord[] = [{ created: 1_000, stats: { cpu: 10 } }]
		const next: TestRecord[] = [{ created: 5_000, stats: { cpu: 20 } }]

		expect(appendData(previous, next)).toEqual([...previous, ...next])
	})

	test("preserves explicit missing telemetry records", () => {
		const missing: TestRecord = { created: null, stats: null }

		expect(appendData([], [missing])).toEqual([missing])
	})

	test("converts PocketBase timestamps to milliseconds", () => {
		const record: TestRecord = { created: "2026-08-22 06:13:14.256Z", stats: { cpu: 20 } }

		const result = appendData([], [record])

		expect(result[0]?.created).toBe(new Date("2026-08-22 06:13:14.256Z").getTime())
	})
})
