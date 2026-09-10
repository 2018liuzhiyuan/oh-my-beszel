import type { AlertRecord } from "@/types"

export const DEFAULT_ALERT_SNOOZE_MINUTES = 60
export const MAX_ALERT_SNOOZE_MINUTES = 525_600

type BrowserStorage = Pick<Storage, "getItem" | "setItem">

type AlertIdentity = Pick<AlertRecord, "system" | "name">

export function alertSnoozeKey(alert: AlertIdentity): string {
	return `${encodeURIComponent(alert.system)}:${encodeURIComponent(alert.name)}`
}

export function normalizeAlertSnoozeMinutes(minutes: number | undefined): number {
	if (!Number.isFinite(minutes) || (minutes ?? 0) < 1) {
		return DEFAULT_ALERT_SNOOZE_MINUTES
	}
	return Math.min(Math.trunc(minutes as number), MAX_ALERT_SNOOZE_MINUTES)
}

export function getAlertSnoozedUntil(now: number, minutes: number | undefined): number {
	return now + normalizeAlertSnoozeMinutes(minutes) * 60_000
}

export function isAlertSnoozed(snoozedUntil: number | undefined, now = Date.now()): boolean {
	return Number.isFinite(snoozedUntil) && (snoozedUntil as number) > now
}

export function pruneAlertSnoozes(
	snoozes: Record<string, number> | undefined,
	allowedKeys: ReadonlySet<string>,
	now = Date.now()
): Record<string, number> {
	return Object.fromEntries(
		Object.entries(snoozes ?? {}).filter(
			([key, snoozedUntil]) => allowedKeys.has(key) && isAlertSnoozed(snoozedUntil, now)
		)
	)
}

export function alertSnoozesStorageKey(userId: string | undefined): string {
	return `besz-alert-snoozes:${encodeURIComponent(userId || "anonymous")}:v1`
}

export function alertSnoozeMinutesStorageKey(userId: string | undefined): string {
	return `besz-alert-snooze-minutes:${encodeURIComponent(userId || "anonymous")}:v1`
}

export function readAlertSnoozes(storage: BrowserStorage, key: string): Record<string, number> {
	try {
		const parsed: unknown = JSON.parse(storage.getItem(key) ?? "{}")
		if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) return {}
		return Object.fromEntries(
			Object.entries(parsed).filter((entry): entry is [string, number] => Number.isFinite(entry[1]))
		)
	} catch {
		return {}
	}
}

export function writeAlertSnoozes(storage: BrowserStorage, key: string, snoozes: Record<string, number>): void {
	try {
		storage.setItem(key, JSON.stringify(snoozes))
	} catch {}
}

export function readAlertSnoozeMinutes(storage: BrowserStorage, key: string): number {
	return normalizeAlertSnoozeMinutes(Number(storage.getItem(key)))
}

export function writeAlertSnoozeMinutes(storage: BrowserStorage, key: string, minutes: number): void {
	try {
		storage.setItem(key, String(normalizeAlertSnoozeMinutes(minutes)))
	} catch {}
}
