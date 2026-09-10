import { describe, expect, mock, test } from "bun:test"
import { renderToStaticMarkup } from "react-dom/server"

mock.module("@/lib/utils", () => ({
	cn: (...inputs: readonly unknown[]) => inputs.filter((input): input is string => typeof input === "string").join(" "),
}))

const { AlertDismissButton } = await import("./alert-dismiss-button")

describe("AlertDismissButton", () => {
	test("renders a centered theme-aware text cross without inherited alert padding", () => {
		// Given / When
		const markup = renderToStaticMarkup(<AlertDismissButton label="Dismiss alert" onDismiss={() => {}} />)

		// Then
		expect(markup).toContain('aria-label="Dismiss alert"')
		expect(markup).toContain(">×</span>")
		expect(markup).not.toContain("❌")
		expect(markup).toContain("!p-0")
		expect(markup).toContain("inline-grid")
		expect(markup).toContain("size-full")
		expect(markup).toContain("place-items-center")
		expect(markup).toContain("text-foreground")
		expect(markup).toContain("text-[0.8em]")
		expect(markup).not.toContain("text-white")
	})
})
