import { describe, expect, test } from "bun:test"
import type { Messages } from "@lingui/core"
import { readFileSync } from "node:fs"
import { messages as englishMessages } from "@/locales/en/en"
import { messages as simplifiedChineseMessages } from "@/locales/zh-CN/zh-CN"
import { messages as hongKongChineseMessages } from "@/locales/zh-HK/zh-HK"
import { messages as traditionalChineseMessages } from "@/locales/zh/zh"

function translationFor(messages: Messages, source: string) {
	const messageId = Object.entries(englishMessages).find(([, value]) => readCompiledText(value) === source)?.[0]
	if (!messageId) {
		throw new Error(`English catalog is missing ${source}`)
	}
	return readCompiledText(messages[messageId])
}

function readCompiledText(message: Messages[string]) {
	return Array.isArray(message) && message.length === 1 && typeof message[0] === "string" ? message[0] : message
}

describe("Chinese localization", () => {
	test("简体中文目录没有空翻译", () => {
		const catalog = readFileSync(new URL("../locales/zh-CN/zh-CN.po", import.meta.url), "utf8")
		const untranslated = catalog
			.split(/\r?\n\r?\n/)
			.map((entry) => ({
				id: entry.match(/^msgid "(.+)"$/m)?.[1],
				empty: /^msgstr ""$/m.test(entry),
			}))
			.filter((entry) => entry.id && entry.empty)
			.map((entry) => entry.id)

		expect(untranslated).toEqual([])
	})

	test.each([
		["简体中文", simplifiedChineseMessages, "切换主题"],
		["香港繁体中文", hongKongChineseMessages, "切換主題"],
		["台湾繁体中文", traditionalChineseMessages, "切換主題"],
	] as const)("%s translates the theme switch tooltip", (_locale, messages, expected) => {
		expect(translationFor(messages, "Switch theme")).toBe(expected)
	})

	test.each([
		["BIOS version", "BIOS 版本"],
		["BMC firmware", "BMC 固件"],
		["Copy public key", "复制公钥"],
		["CPU", "CPU"],
		["Fans", "风扇"],
		["Free GPU Memory", "可用 GPU 显存"],
		["GPU", "GPU"],
		["Hardware event log (IPMI SEL)", "硬件事件日志 (IPMI SEL)"],
		["IP addresses", "IP 地址"],
		["Network interfaces (nominal link speed)", "网络接口（标称链路速度）"],
		["Public key", "公钥"],
		["System fan speeds (RPM)", "系统风扇转速 (RPM)"],
		["Total time spent on read/write (can exceed 100%)", "读写总耗时（可超过 100%）"],
		["Triggers when free VRAM on any GPU stays above a threshold", "任一 GPU 的可用显存持续高于阈值时触发"],
		["Unable to load configuration", "无法加载配置"],
	] as const)("简体中文明确翻译 %s", (source, expected) => {
		expect(translationFor(simplifiedChineseMessages, source)).toBe(expected)
	})
})
