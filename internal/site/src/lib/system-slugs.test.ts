import { describe, expect, test } from "bun:test"
import { buildSystemRoutes } from "./system-slugs"

describe("system name routes", () => {
	test.each(["16.7", "Pro6000_2", "GPU-01", "训练服务器"])("preserves readable hostname %s", (name) => {
		const routes = buildSystemRoutes([{ id: "record1", name }])
		expect(routes.byId.get("record1")).toBe(name)
		expect(routes.bySlug.get(name)?.id).toBe("record1")
	})
	test("converts URL delimiters and whitespace into hyphens", () => {
		const routes = buildSystemRoutes([{ id: "record1", name: " GPU Server / A?#% " }])
		expect(routes.byId.get("record1")).toBe("GPU-Server-A")
	})
	test("distinguishes names that normalize to the same slug regardless of input order", () => {
		const systems = [
			{ id: "record1", name: "GPU / A" },
			{ id: "record2", name: "GPU-A" },
		]
		const routes = buildSystemRoutes(systems)
		expect(routes.byId.get("record1")).toBe("GPU-A~record1")
		expect(routes.byId.get("record2")).toBe("GPU-A~record2")
		expect(buildSystemRoutes([...systems].reverse()).byId.get("record1")).toBe("GPU-A~record1")
		for (const system of systems) expect(routes.bySlug.get(routes.byId.get(system.id) ?? "")?.id).toBe(system.id)
	})
	test("keeps legacy IDs unambiguous when a hostname equals another record ID", () => {
		const routes = buildSystemRoutes([
			{ id: "record1", name: "record2" },
			{ id: "record2", name: "GPU" },
		])
		expect(routes.byId.get("record1")).toBe("record2~record1")
	})
	test.each(["", "???", ".", ".."])('uses a safe fallback for "%s"', (name) => {
		expect(buildSystemRoutes([{ id: "record1", name }]).byId.get("record1")).toBe("system~record1")
	})
})

describe("extreme hostname URL validity", () => {
	test.each([
		["[2001:db8::1]", "2001-db8-1"],
		["100% GPU?#", "100-GPU"],
		["gpu%2Fadmin", "gpu-2Fadmin"],
		["../../admin", "..-..-admin"],
		["//evil.example/a", "evil.example-a"],
		["https://example.com/a", "https-example.com-a"],
		["gpu\\node", "gpu-node"],
		["GPU\u0000A", "GPU-A"],
		["gpu\u202Eabc", "gpu-abc"],
		["ＧＰＵ＿０１", "GPU_01"],
		["Cafe\u0301", "Café"],
		["𠮷野服务器", "𠮷野服务器"],
		["🚀", "system~record1"],
		["__proto__", "__proto__"],
		["constructor", "constructor"],
		["toString", "toString"],
		["a~record1", "a-record1"],
	])("keeps %s in a single local path segment", (name, expected) => {
		const routes = buildSystemRoutes([{ id: "record1", name }])
		const slug = routes.byId.get("record1") ?? ""
		const url = new URL(`/system/${encodeURIComponent(slug)}`, "http://127.0.0.1:8090")
		expect(slug).toBe(expected)
		expect(url.origin).toBe("http://127.0.0.1:8090")
		expect(url.search + url.hash).toBe("")
		expect(url.pathname.split("/")).toHaveLength(3)
		expect(routes.bySlug.get(decodeURIComponent(url.pathname.split("/")[2] ?? ""))?.id).toBe("record1")
	})
	test.each(["x".repeat(4000), "训练服务器".repeat(200), "𠮷".repeat(100)])(
		"limits a very long name without splitting Unicode characters",
		(name) => {
			const slug = buildSystemRoutes([{ id: "record1", name }]).byId.get("record1") ?? ""
			expect(slug).toBe(`${Array.from(name).slice(0, 80).join("")}~record1`)
			expect(encodeURIComponent(slug).length).toBeLessThan(1100)
		}
	)
	test("keeps identical names and normalized Unicode names separately addressable", () => {
		const systems = [
			{ id: "one", name: "GPU" },
			{ id: "two", name: "GPU" },
			{ id: "three", name: "ＧＰＵ" },
		]
		const routes = buildSystemRoutes(systems)
		expect(new Set(routes.byId.values()).size).toBe(3)
		for (const system of systems) expect(routes.bySlug.get(routes.byId.get(system.id) ?? "")?.id).toBe(system.id)
	})
})

describe("hostname route invariants", () => {
	test("keeps case-sensitive names distinct", () => {
		const routes = buildSystemRoutes([
			{ id: "one", name: "GPU" },
			{ id: "two", name: "gpu" },
		])
		expect(routes.bySlug.get("GPU")?.id).toBe("one")
		expect(routes.bySlug.get("gpu")?.id).toBe("two")
	})
	test("disambiguates long names with identical truncated prefixes", () => {
		const names = [`${"x".repeat(80)}a`, `${"x".repeat(80)}b`, "x".repeat(80)]
		const systems = names.map((name, i) => ({ id: `record${i}`, name }))
		const routes = buildSystemRoutes(systems)
		expect(new Set(routes.byId.values()).size).toBe(3)
		for (const system of systems) expect(routes.bySlug.get(routes.byId.get(system.id) ?? "")?.id).toBe(system.id)
	})
	test("round-trips a mixed Unicode and punctuation corpus without changing origin", () => {
		const pieces = [
			"",
			"GPU",
			"训练",
			"𠮷",
			"é",
			"e\u0301",
			"🚀",
			"/",
			"?",
			"#",
			"%2F",
			"..",
			"~",
			" ",
			"\u0000",
			"\u202E",
		]
		const systems = pieces.flatMap((left, i) => pieces.map((right, j) => ({ id: `r${i}x${j}`, name: left + right })))
		const routes = buildSystemRoutes(systems)
		expect(routes.bySlug.size).toBe(systems.length)
		for (const system of systems) {
			const slug = routes.byId.get(system.id) ?? ""
			const url = new URL(`/system/${encodeURIComponent(slug)}`, "http://localhost:8090")
			expect(url.origin).toBe("http://localhost:8090")
			expect(url.search + url.hash).toBe("")
			expect(url.pathname.split("/")).toHaveLength(3)
			expect(routes.bySlug.get(decodeURIComponent(url.pathname.slice(8)))?.id).toBe(system.id)
		}
	})
})
