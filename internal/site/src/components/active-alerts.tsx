import { alertInfo } from "@/lib/alerts"
import {
	alertSnoozeKey,
	alertSnoozeMinutesStorageKey,
	alertSnoozesStorageKey,
	getAlertSnoozedUntil,
	isAlertSnoozed,
	pruneAlertSnoozes,
	readAlertSnoozeMinutes,
	readAlertSnoozes,
	writeAlertSnoozes,
} from "@/lib/alert-snooze"
import { pb } from "@/lib/api"
import { $alerts, $alertsLoaded, $allSystemsById } from "@/lib/stores"
import type { AlertRecord } from "@/types"
import { t } from "@lingui/core/macro"
import { Plural, Trans } from "@lingui/react/macro"
import { useStore } from "@nanostores/react"
import { getPagePath } from "@nanostores/router"
import { useEffect, useMemo, useState } from "react"
import { AlertDismissButton } from "./alert-dismiss-button"
import { $router, Link } from "./router"
import { Alert, AlertTitle, AlertDescription } from "./ui/alert"
import { Card, CardHeader, CardTitle, CardContent } from "./ui/card"

export const ActiveAlerts = () => {
	const alerts = useStore($alerts)
	const alertsLoaded = useStore($alertsLoaded)
	const systems = useStore($allSystemsById)
	const [now, setNow] = useState(Date.now)
	const userId = pb.authStore.record?.id
	const snoozesStorageKey = alertSnoozesStorageKey(userId)
	const minutesStorageKey = alertSnoozeMinutesStorageKey(userId)
	const [snoozes, setSnoozes] = useState(() => readAlertSnoozes(localStorage, snoozesStorageKey))

	const alertKeys = useMemo(() => {
		const keys = new Set<string>()
		for (const systemAlerts of Object.values(alerts)) {
			for (const alert of systemAlerts.values()) keys.add(alertSnoozeKey(alert))
		}
		return keys
	}, [alerts])

	const activeAlerts = useMemo(() => {
		const activeAlerts: AlertRecord[] = []

		for (const systemId of Object.keys(alerts)) {
			for (const alert of alerts[systemId].values()) {
				const snoozedUntil = snoozes[alertSnoozeKey(alert)]
				if (alert.triggered && alert.name in alertInfo && !isAlertSnoozed(snoozedUntil, now)) {
					activeAlerts.push(alert)
				}
			}
		}

		return activeAlerts
	}, [alerts, now, snoozes])

	useEffect(() => {
		setSnoozes(readAlertSnoozes(localStorage, snoozesStorageKey))
	}, [snoozesStorageKey])

	useEffect(() => {
		if (!alertsLoaded) return
		setSnoozes((currentSnoozes) => {
			const prunedSnoozes = pruneAlertSnoozes(currentSnoozes, alertKeys, now)
			writeAlertSnoozes(localStorage, snoozesStorageKey, prunedSnoozes)
			return prunedSnoozes
		})
	}, [alertKeys, alertsLoaded, now, snoozesStorageKey])

	useEffect(() => {
		const nextExpiry = Object.values(snoozes)
			.filter((snoozedUntil) => isAlertSnoozed(snoozedUntil, now))
			.sort((a, b) => a - b)[0]
		if (!nextExpiry) return
		const timeout = window.setTimeout(() => setNow(Date.now()), Math.min(Math.max(0, nextExpiry - now), 2_147_483_647))
		return () => window.clearTimeout(timeout)
	}, [now, snoozes])

	function dismissAlert(alert: AlertRecord) {
		const dismissedAt = Date.now()
		const key = alertSnoozeKey(alert)
		setSnoozes((currentSnoozes) => {
			const nextSnoozes = pruneAlertSnoozes(currentSnoozes, alertKeys, dismissedAt)
			nextSnoozes[key] = getAlertSnoozedUntil(dismissedAt, readAlertSnoozeMinutes(localStorage, minutesStorageKey))
			writeAlertSnoozes(localStorage, snoozesStorageKey, nextSnoozes)
			return nextSnoozes
		})
	}

	if (activeAlerts.length === 0) {
		return null
	}
	return (
		<Card>
			<CardHeader className="pb-4 px-2 sm:px-6 max-sm:pt-5 max-sm:pb-1">
				<div className="px-2 sm:px-1">
					<CardTitle>
						<Trans>Alerts</Trans>
					</CardTitle>
				</div>
			</CardHeader>
			<CardContent className="max-sm:p-2">
				{activeAlerts.length > 0 && (
					<div className="grid sm:grid-cols-2 lg:grid-cols-3 2xl:grid-cols-4 gap-3">
						{activeAlerts.map((alert) => {
							const info = alertInfo[alert.name as keyof typeof alertInfo]
							return (
								<Alert
									key={alert.id}
									className="hover:-translate-y-px duration-200 bg-transparent border-foreground/10 hover:shadow-md shadow-black/5 pe-11"
								>
									<info.icon className="h-4 w-4" />
									<AlertTitle>
										{systems[alert.system]?.name} {info.name()}
									</AlertTitle>
									<AlertDescription>
										{alert.name === "Status" ? (
											<Trans>Connection is down</Trans>
										) : info.invert ? (
											<Trans>
												Below {alert.value}
												{info.unit} in last <Plural value={alert.min} one="# minute" other="# minutes" />
											</Trans>
										) : (
											<Trans>
												Exceeds {alert.value}
												{info.unit} in last <Plural value={alert.min} one="# minute" other="# minutes" />
											</Trans>
										)}
									</AlertDescription>
									<Link
										href={getPagePath($router, "system", { id: systems[alert.system]?.id })}
										className="absolute inset-0 w-full h-full"
										aria-label="View system"
									></Link>
									<AlertDismissButton label={t`Dismiss alert`} onDismiss={() => dismissAlert(alert)} />
								</Alert>
							)
						})}
					</div>
				)}
			</CardContent>
		</Card>
	)
}
