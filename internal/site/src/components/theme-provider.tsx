import { createContext, useCallback, useContext, useEffect, useMemo, useState } from "react"

type Theme = "dark" | "light" | "system"

type ThemeProviderProps = {
	readonly children: React.ReactNode
	readonly defaultTheme?: Theme
	readonly storageKey?: string
}

type ThemeProviderState = {
	readonly theme: Theme
	readonly setTheme: (theme: Theme) => void
}

const initialState: ThemeProviderState = {
	theme: "system",
	setTheme: () => null,
}

const ThemeProviderContext = createContext<ThemeProviderState>(initialState)

export function ThemeProvider({
	children,
	defaultTheme = "system",
	storageKey = "ui-theme",
	...props
}: ThemeProviderProps) {
	const [theme, setTheme] = useState<Theme>(() => {
		const storedTheme = localStorage.getItem(storageKey)
		return storedTheme === "dark" || storedTheme === "light" || storedTheme === "system" ? storedTheme : defaultTheme
	})

	useEffect(() => {
		const root = window.document.documentElement

		root.classList.remove("light", "dark")

		if (theme === "system") {
			const systemTheme = window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light"

			root.classList.add(systemTheme)
			return
		}

		root.classList.add(theme)
	}, [theme])

	const updateTheme = useCallback(
		(nextTheme: Theme) => {
			localStorage.setItem(storageKey, nextTheme)
			setTheme(nextTheme)
		},
		[storageKey]
	)
	const value = useMemo(() => ({ theme, setTheme: updateTheme }), [theme, updateTheme])

	return (
		<ThemeProviderContext.Provider {...props} value={value}>
			{children}
		</ThemeProviderContext.Provider>
	)
}

export const useTheme = () => useContext(ThemeProviderContext)
