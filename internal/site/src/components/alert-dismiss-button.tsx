import { Button } from "./ui/button"
import { Tooltip, TooltipContent, TooltipTrigger } from "./ui/tooltip"

type AlertDismissButtonProps = {
	readonly label: string
	readonly onDismiss: () => void
}

export function AlertDismissButton({ label, onDismiss }: AlertDismissButtonProps) {
	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<Button
					type="button"
					variant="ghost"
					size="icon"
					className="absolute end-2 top-2 z-10 size-7 !p-0 text-foreground hover:text-foreground"
					aria-label={label}
					onClick={onDismiss}
				>
					<span
						aria-hidden="true"
						className="inline-grid size-full place-items-center font-sans text-[0.8em] leading-none text-foreground"
					>
						×
					</span>
				</Button>
			</TooltipTrigger>
			<TooltipContent side="top">{label}</TooltipContent>
		</Tooltip>
	)
}
