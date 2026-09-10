import { Trans } from "@lingui/react/macro"
import { Button } from "./ui/button"
import { Checkbox } from "./ui/checkbox"
import { Label } from "./ui/label"
import type { SSHHost, SSHManagedSystem } from "./ssh-host-actions"

type SSHHostListProps = {
	readonly hosts: readonly SSHHost[]
	readonly systems: readonly SSHManagedSystem[]
	readonly selected: ReadonlySet<string>
	readonly loading: boolean
	readonly onSelectionChange: (selected: Set<string>) => void
}

export function SSHHostList({ hosts, systems, selected, loading, onSelectionChange }: SSHHostListProps) {
	const systemsByName = new Map(systems.map((system) => [system.name.toLowerCase(), system]))
	const selectableNames: string[] = []
	for (const host of hosts) {
		const system = systemsByName.get(host.name.toLowerCase())
		if (!system || system.ssh_config) selectableNames.push(host.name)
	}

	function toggle(name: string, checked: boolean) {
		const next = new Set(selected)
		if (checked) next.add(name)
		else next.delete(name)
		onSelectionChange(next)
	}

	return (
		<div className="grid gap-2 min-h-0">
			<div className="flex items-center justify-between gap-3">
				<Label>
					<Trans>Remote hosts</Trans>
				</Label>
				<div className="flex gap-1">
					<Button
						type="button"
						variant="ghost"
						size="sm"
						className="h-11 sm:h-9"
						onClick={() => onSelectionChange(new Set(selectableNames))}
					>
						<Trans>Select all</Trans>
					</Button>
					<Button
						type="button"
						variant="ghost"
						size="sm"
						className="h-11 sm:h-9"
						onClick={() => onSelectionChange(new Set())}
					>
						<Trans>Clear</Trans>
					</Button>
				</div>
			</div>
			<div className="max-h-64 overflow-y-auto rounded-md border p-2 min-h-24">
				{loading && (
					<p className="p-3 text-sm text-muted-foreground">
						<Trans>Loading SSH hosts...</Trans>
					</p>
				)}
				{!loading && hosts.length === 0 && (
					<p className="p-3 text-sm text-muted-foreground">
						<Trans>No concrete hosts found.</Trans>
					</p>
				)}
				{hosts.map((host) => {
					const system = systemsByName.get(host.name.toLowerCase())
					const isManaged = Boolean(system?.ssh_config)
					const isBlocked = Boolean(system && !isManaged)
					return (
						<Label key={host.name} className="flex items-center gap-3 rounded-md px-2 py-2 hover:bg-accent/60">
							<Checkbox
								checked={!isBlocked && selected.has(host.name)}
								disabled={isBlocked}
								onCheckedChange={(checked) => toggle(host.name, checked === true)}
							/>
							<span className="min-w-0 flex-1">
								<span className="block truncate">{host.name}</span>
								<span className="block truncate text-xs font-normal text-muted-foreground">{host.hostName}</span>
							</span>
							{isManaged && (
								<span className="text-xs font-normal text-muted-foreground" title={system?.ssh_config}>
									<Trans>SSH system</Trans>
								</span>
							)}
							{isBlocked && (
								<span className="text-xs font-normal text-muted-foreground">
									<Trans>Name already used</Trans>
								</span>
							)}
						</Label>
					)
				})}
			</div>
		</div>
	)
}
