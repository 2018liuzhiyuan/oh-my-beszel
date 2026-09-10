import { useStore } from "@nanostores/react"
import { getPagePath } from "@nanostores/router"
import { computed } from "nanostores"
import { useEffect, useRef } from "react"
import { $router } from "@/components/router"
import { $systems } from "./stores"
import { buildSystemRoutes } from "./system-slugs"

const $systemRoutes = computed($systems, buildSystemRoutes)

export function getSystemPath(id: string) {
	return getPagePath($router, "system", { id: $systemRoutes.get().byId.get(id) ?? id })
}

export function useSystemRouteId(route: string) {
	const routes = useStore($systemRoutes)
	const bound = useRef({ route: "", id: "" })
	const suffixedId = route.slice(route.lastIndexOf("~") + 1)
	const id =
		bound.current.route === route && routes.byId.has(bound.current.id)
			? bound.current.id
			: routes.byId.has(route)
				? route
				: (routes.bySlug.get(route)?.id ??
					(routes.byId.has(suffixedId) ? suffixedId : undefined) ??
					$systems.get().find((system) => system.name === route)?.id ??
					"")
	bound.current = { route, id }
	useEffect(() => {
		if (!id) return
		const current = $router.get()
		const path = getSystemPath(id)
		if (current?.route !== "system" || current.params.id !== route || current.path === path) return
		const url = new URL(window.location.href)
		url.pathname = path
		window.history.replaceState(null, "", url)
		window.dispatchEvent(new PopStateEvent("popstate"))
	}, [route, id, routes])
	return id
}
