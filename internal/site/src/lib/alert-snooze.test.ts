import { describe, expect, test } from "bun:test"
import {
	DEFAULT_ALERT_SNOOZE_MINUTES,
	alertSnoozeKey,
	alertSnoozeMinutesStorageKey,
	alertSnoozesStorageKey,
	getAlertSnoozedUntil,
	isAlertSnoozed,
	pruneAlertSnoozes,
	readAlertSnoozeMinutes,
	readAlertSnoozes,
	writeAlertSnoozeMinutes,
	writeAlertSnoozes,
} from "./alert-snooze"

function memoryStorage() {
	const values = new Map<string, string>()
	return {
		getItem: (key: string) => values.get(key) ?? null,
		setItem: (key: string, value: string) => values.set(key, value),
	}
}

describe("alert snooze", () => {
	test("defaults dismissed alerts to a 60 minute snooze", () => {
		expect(DEFAULT_ALERT_SNOOZE_MINUTES).toBe(60)
		expect(getAlertSnoozedUntil(1_000, undefined)).toBe(3_601_000)
	})

	test("identifies an alert by system and event", () => {
		expect(alertSnoozeKey({ system: "server:a", name: "GPU Usage" })).toBe("server%3Aa:GPU%20Usage")
		expect(alertSnoozeKey({ system: "server:b", name: "GPU Usage" })).not.toBe(
			alertSnoozeKey({ system: "server:a", name: "GPU Usage" })
		)
		expect(alertSnoozeKey({ system: "server:a", name: "Memory" })).not.toBe(
			alertSnoozeKey({ system: "server:a", name: "GPU Usage" })
		)
	})

	test("honors a custom whole-minute snooze interval", () => {
		expect(getAlertSnoozedUntil(1_000, 15)).toBe(901_000)
		expect(getAlertSnoozedUntil(1_000, 15.9)).toBe(901_000)
	})

	test("restores the alert at the exact expiry time", () => {
		expect(isAlertSnoozed(3_601_000, 3_600_999)).toBe(true)
		expect(isAlertSnoozed(3_601_000, 3_601_000)).toBe(false)
	})

	test("keeps at most one entry for each currently configured alert event", () => {
		expect(
			pruneAlertSnoozes(
				{
					active: 5_000,
					expired: 4_000,
					invalid: Number.NaN,
					deletedEvent: 8_000,
				},
				new Set(["active", "expired", "invalid"]),
				4_000
			)
		).toEqual({ active: 5_000 })
	})

	test("stores snoozes and the interval locally per user", () => {
		const storage = memoryStorage()
		const snoozesKey = alertSnoozesStorageKey("user:a")
		const minutesKey = alertSnoozeMinutesStorageKey("user:a")

		writeAlertSnoozes(storage, snoozesKey, { alert: 5_000 })
		writeAlertSnoozeMinutes(storage, minutesKey, 15)

		expect(readAlertSnoozes(storage, snoozesKey)).toEqual({ alert: 5_000 })
		expect(readAlertSnoozeMinutes(storage, minutesKey)).toBe(15)
		expect(alertSnoozesStorageKey("user:b")).not.toBe(snoozesKey)
	})

	test("ignores malformed local storage and falls back to 60 minutes", () => {
		const storage = memoryStorage()
		storage.setItem("snoozes", "not-json")
		storage.setItem("minutes", "0")

		expect(readAlertSnoozes(storage, "snoozes")).toEqual({})
		expect(readAlertSnoozeMinutes(storage, "minutes")).toBe(60)
	})
})
