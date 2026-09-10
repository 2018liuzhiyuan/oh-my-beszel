type MeterBarProps = {
	value: number
	fillClassName: string
}

export function MeterBar({ value, fillClassName }: MeterBarProps) {
	return (
		// biome-ignore lint/a11y/useSemanticElements: Native meter rendering would replace the threshold-colored custom fill.
		<span
			className="flex-1 min-w-8 grid bg-muted h-[1em] rounded-sm overflow-hidden"
			role="meter"
			aria-valuemin={0}
			aria-valuemax={100}
			aria-valuenow={value}
		>
			<span className={`h-full ${fillClassName}`} style={{ width: `${value}%` }} />
		</span>
	)
}
