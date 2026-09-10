type SystemName = { readonly id: string; readonly name: string }

export function buildSystemRoutes<T extends SystemName>(systems: readonly T[]) {
	const byId = new Map<string, string>()
	const bySlug = new Map<string, T>()
	const names = new Map<string, string>()
	const shortened = new Set<string>()
	const counts = new Map<string, number>()
	const ids = new Set(systems.map((system) => system.id))
	for (const system of systems) {
		const name = system.name
			.normalize("NFKC")
			.trim()
			.replace(/[^\p{L}\p{N}._-]+/gu, "-")
			.replace(/^-+|-+$/g, "")
		const letters = Array.from(name)
		if (letters.length > 80) shortened.add(system.id)
		const slug = name === "." || name === ".." ? "" : letters.slice(0, 80).join("")
		names.set(system.id, slug)
		counts.set(slug, (counts.get(slug) ?? 0) + 1)
	}
	for (const system of systems) {
		const name = names.get(system.id) ?? ""
		const needsId = shortened.has(system.id) || !name || (counts.get(name) ?? 0) > 1 || ids.has(name)
		const slug = needsId ? `${name || "system"}~${system.id}` : name
		byId.set(system.id, slug)
		bySlug.set(slug, system)
	}
	return { byId, bySlug }
}
