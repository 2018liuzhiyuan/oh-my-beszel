import { Trans } from "@lingui/react/macro"
import { WifiOffIcon } from "lucide-react"
import { useCallback, useEffect, useState } from "react"
import { cn } from "@/lib/utils"

// how often to probe the hub between UI activity; kept small enough that an
// outage is noticed quickly but large enough to be negligible load
const HEALTH_CHECK_INTERVAL_MS = 20_000
// a single missed probe is treated as transient; two in a row surface the banner
const DISCONNECTED_AFTER_FAILURES = 2

type HubHealthState = "ok" | "disconnected"

// probeHub pings the unauthenticated health endpoint. Deliberately uses plain
// fetch with document.baseURI instead of the PocketBase client so this module
// stays out of the lib/api import cycle.
async function probeHub(): Promise<boolean> {
	try {
		const response = await fetch(new URL("api/health", document.baseURI))
		return response.ok
	} catch {
		return false
	}
}

/**
 * HubHealth watches hub reachability while the app is open. When the hub
 * stops responding, a non-blocking banner explains the stale data; once it is
 * back, the systems list is refreshed so the UI catches up immediately
 * instead of waiting for the next realtime event.
 */
export function HubHealth() {
	const [state, setState] = useState<HubHealthState>("ok")

	const check = useCallback(() => probeHub(), [])

	useEffect(() => {
		let failures = 0
		let disposed = false
		let timer: ReturnType<typeof setInterval> | undefined

		const probe = async () => {
			const healthy = await check()
			if (disposed) return
			if (healthy) {
				const wasDisconnected = failures >= DISCONNECTED_AFTER_FAILURES
				failures = 0
				if (wasDisconnected) {
					setState("ok")
					// realtime may have missed events while the hub was down
					const systemsManager = await import("@/lib/systemsManager")
					if (!disposed) {
						systemsManager.refresh().catch(console.error)
					}
				}
			} else {
				failures += 1
				if (failures >= DISCONNECTED_AFTER_FAILURES) {
					setState("disconnected")
				}
			}
		}

		probe().catch(console.error)
		timer = setInterval(probe, HEALTH_CHECK_INTERVAL_MS)
		// the OS "online" event often precedes our interval after a network switch
		window.addEventListener("online", probe)

		return () => {
			disposed = true
			if (timer) clearInterval(timer)
			window.removeEventListener("online", probe)
		}
	}, [check])

	if (state !== "disconnected") return null

	return (
		<div
			data-testid="hub-health-banner"
			role="alert"
			aria-live="polite"
			className={cn(
				"fixed inset-x-0 bottom-4 z-50 mx-auto w-fit max-w-[90%]",
				"flex items-center gap-2 rounded-full bg-destructive text-destructive-foreground",
				"px-4 py-2 text-sm shadow-lg select-none"
			)}
		>
			<WifiOffIcon className="size-4 shrink-0" />
			<Trans>Connection to the hub lost. Reconnecting…</Trans>
		</div>
	)
}
