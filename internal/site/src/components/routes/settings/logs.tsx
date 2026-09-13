import { t } from "@lingui/core/macro"
import { Trans } from "@lingui/react/macro"
import { redirectPage } from "@nanostores/router"
import { RefreshCwIcon } from "lucide-react"
import { useCallback, useEffect, useState } from "react"
import { $router } from "@/components/router"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Separator } from "@/components/ui/separator"
import { toast } from "@/components/ui/use-toast"
import { isAdmin, pb } from "@/lib/api"
import { cn } from "@/lib/utils"

interface HubLogEntry {
	readonly created: string
	readonly level: number
	readonly message: string
	readonly data: Record<string, unknown> | null
}

const LEVEL_FILTER_VALUES = [0, 4, 8] as const

function levelBadgeVariant(level: number): "success" | "warning" | "destructive" | "secondary" {
	if (level >= 8) return "destructive"
	if (level >= 4) return "warning"
	return level >= 0 ? "secondary" : "success"
}

export default function LogsSettings() {
	const [entries, setEntries] = useState<readonly HubLogEntry[]>([])
	const [isLoading, setIsLoading] = useState(true)
	const [minLevel, setMinLevel] = useState<number>(4)
	const [query, setQuery] = useState("")

	if (!isAdmin()) {
		redirectPage($router, "settings", { name: "general" })
	}

	const fetchLogs = useCallback(
		async (signal?: AbortSignal) => {
			try {
				setIsLoading(true)
				const options: Record<string, unknown> = { query: { level: minLevel }, signal }
				const trimmed = query.trim()
				if (trimmed) options.query.q = trimmed
				const res = await pb.send<{ entries: HubLogEntry[] }>("/api/beszel/hub-logs", options)
				if (signal?.aborted) return
				setEntries(res.entries)
			} catch (error: unknown) {
				if (signal?.aborted) return
				toast({
					title: t`Error`,
					description: (error as Error).message,
					variant: "destructive",
				})
			} finally {
				if (!signal?.aborted) setIsLoading(false)
			}
		},
		[minLevel, query]
	)

	useEffect(() => {
		const controller = new AbortController()
		const timer = setTimeout(() => fetchLogs(controller.signal), 250)
		return () => {
			clearTimeout(timer)
			controller.abort()
		}
	}, [fetchLogs])

	return (
		<div>
			<div>
				<h3 className="text-xl font-medium mb-2">
					<Trans>Hub Logs</Trans>
				</h3>
				<p className="text-sm text-muted-foreground leading-relaxed">
					<Trans>
						Recent hub events: connection failures, agent deployments, and other warnings. Older entries are pruned
						automatically; the full history lives in the hub's log output.
					</Trans>
				</p>
			</div>
			<Separator className="my-4" />

			<div className="flex flex-wrap items-center gap-2 mb-3">
				<Input
					type="search"
					className="max-w-56"
					placeholder={t`Filter messages`}
					value={query}
					onChange={(event) => setQuery(event.target.value)}
					aria-label={t`Filter messages`}
				/>
				<div className="flex gap-1">
					{LEVEL_FILTER_VALUES.map((value, index) => (
						<Button
							key={value}
							type="button"
							variant={minLevel === value ? "default" : "outline"}
							size="sm"
							onClick={() => setMinLevel(value)}
						>
							{[t`All`, t`Warn+`, t`Error`][index]}
						</Button>
					))}
				</div>
				<Button
					type="button"
					variant="outline"
					size="icon"
					onClick={() => fetchLogs()}
					disabled={isLoading}
					aria-label={t`Refresh`}
					title={t`Refresh`}
				>
					<RefreshCwIcon className={cn("size-4", isLoading && "animate-spin")} />
				</Button>
			</div>

			<div className={cn("grid gap-1.5", isLoading && entries.length === 0 && "animate-pulse")}>
				{entries.length === 0 && !isLoading ? (
					<p className="text-sm text-muted-foreground">
						<Trans>No log entries at this level.</Trans>
					</p>
				) : (
					entries.map((entry) => <LogRow key={`${entry.created}-${entry.message}`} entry={entry} />)
				)}
			</div>
		</div>
	)
}

function LogRow({ entry }: { entry: HubLogEntry }) {
	const [expanded, setExpanded] = useState(false)
	const dataEntries = Object.entries(entry.data ?? {})
	return (
		<div className="bg-muted/50 rounded-md px-3 py-2 grid gap-1 min-w-0">
			<div className="flex flex-wrap items-center gap-2">
				<Badge variant={levelBadgeVariant(entry.level)} className="uppercase text-[0.65rem]">
					{entry.level >= 8 ? "error" : entry.level >= 4 ? "warn" : entry.level >= 0 ? "info" : "debug"}
				</Badge>
				<span className="text-xs text-muted-foreground tabular-nums">{new Date(entry.created).toLocaleString()}</span>
				<span className="text-sm font-medium break-all">{entry.message}</span>
				{dataEntries.length > 0 && (
					<button
						type="button"
						className="text-xs text-muted-foreground underline"
						onClick={() => setExpanded(!expanded)}
					>
						{expanded ? <Trans>Hide details</Trans> : <Trans>Show details</Trans>}
					</button>
				)}
			</div>
			{expanded && dataEntries.length > 0 && (
				<pre className="text-xs text-muted-foreground font-mono whitespace-pre-wrap break-all">
					{JSON.stringify(entry.data, null, 2)}
				</pre>
			)}
		</div>
	)
}
