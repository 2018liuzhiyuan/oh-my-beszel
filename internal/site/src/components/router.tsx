import { createRouter } from "@nanostores/router"

/**
 * The base path of the application.
 * This is used to prepend the base path to all routes.
 */
export const basePath = globalThis.BESZEL?.BASE_PATH || ""

/**
 * Prepends the base path to the given path.
 * @param path The path to prepend the base path to.
 * @returns The path with the base path prepended.
 */
export const prependBasePath = <Path extends string>(path: Path): Path => (basePath + path).replaceAll("//", "/") as Path

const routes = {
	home: prependBasePath("/"),
	containers: prependBasePath("/containers"),
	smart: prependBasePath("/smart"),
	system: prependBasePath("/system/:id"),
	settings: prependBasePath("/settings/:name?"),
	forgot_password: prependBasePath("/forgot-password"),
	request_otp: prependBasePath("/request-otp"),
} as const

export const $router = createRouter(routes, { links: false })

/** Navigate to url using router
 *  Base path is automatically prepended if serving from subpath
 */
export const navigate = (urlString: string) => {
	$router.open(urlString)
}

export function Link(props: React.AnchorHTMLAttributes<HTMLAnchorElement>) {
	const { href, onClick, ...anchorProps } = props
	return (
		<a
			{...anchorProps}
			href={href}
			onClick={(event) => {
				onClick?.(event)
				if (
					event.defaultPrevented ||
					event.button !== 0 ||
					event.metaKey ||
					event.ctrlKey ||
					event.shiftKey ||
					event.altKey ||
					!href ||
					(anchorProps.target && anchorProps.target !== "_self") ||
					anchorProps.download
				) {
					return
				}
				event.preventDefault()
				navigate(href)
			}}
		></a>
	)
}
