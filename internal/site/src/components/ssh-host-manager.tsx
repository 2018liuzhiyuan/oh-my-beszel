import { Trans, useLingui } from "@lingui/react/macro"
import { useStore } from "@nanostores/react"
import { RefreshCwIcon, SaveIcon, UploadIcon } from "lucide-react"
import { useCallback, useEffect, useMemo, useState } from "react"
import { isReadOnlyUser, pb } from "@/lib/api"
import { $publicKey, $systems, $userSettings } from "@/lib/stores"
import { cn } from "@/lib/utils"
import { Button } from "./ui/button"
import { DialogFooter } from "./ui/dialog"
import { Input } from "./ui/input"
import { Label } from "./ui/label"
import { getSSHHostActionTargets, type SSHHost } from "./ssh-host-actions"
import { SSHHostList } from "./ssh-host-list"

type SSHHostsResponse = {
	readonly path: string
	readonly hosts: readonly SSHHost[]
}

// persistSSHConfigPath remembers the last loaded config path in the user's
// settings so the next add-system session (on any device using this account)
// starts from it instead of the auto-detected default.
async function persistSSHConfigPath(path: string) {
	const current = $userSettings.get()
	if (current.sshConfigPath === path) return
	try {
		const record = await pb.collection("user_settings").getFirstListItem("", { fields: "id,settings" })
		await pb.collection("user_settings").update(record.id, {
			settings: { ...record.settings, sshConfigPath: path },
		})
		$userSettings.set({ ...current, sshConfigPath: path })
	} catch (error) {
		console.error("Failed to remember SSH config path", error)
	}
}

export function SSHHostManager() {
	const { t } = useLingui()
	const systems = useStore($systems)
	const [path, setPath] = useState("")
	const [loadedPath, setLoadedPath] = useState("")
	const [port, setPort] = useState("45876")
	const [discoveredHosts, setDiscoveredHosts] = useState<readonly SSHHost[]>([])
	const [selected, setSelected] = useState<Set<string>>(new Set())
	const [loading, setLoading] = useState(false)
	const [importing, setImporting] = useState(false)
	const [updating, setUpdating] = useState(false)
	const [feedback, setFeedback] = useState("")
	const hosts = discoveredHosts
	const actionTargets = useMemo(() => getSSHHostActionTargets(hosts, systems, selected), [hosts, systems, selected])
	const updatePathSystems = useMemo(
		() => actionTargets.managedSystems.filter((system) => system.ssh_config !== loadedPath),
		[actionTargets.managedSystems, loadedPath]
	)

	const loadHosts = useCallback(
		async (customPath: string, signal?: AbortSignal) => {
			setLoading(true)
			setFeedback("")
			try {
				const options = customPath.trim() ? { query: { path: customPath.trim() }, signal } : { signal }
				const response = await pb.send<SSHHostsResponse>("/api/beszel/ssh-hosts", options)
				if (signal?.aborted) return
				const existingNames = new Set($systems.get().map(({ name }) => name.toLowerCase()))
				setPath(response.path)
				setLoadedPath(response.path)
				setDiscoveredHosts(response.hosts)
				if (response.path) {
					persistSSHConfigPath(response.path).catch(console.error)
				}
				const newNames = new Set<string>()
				for (const host of response.hosts) {
					if (!existingNames.has(host.name.toLowerCase())) newNames.add(host.name)
				}
				setSelected(newNames)
			} catch (error: unknown) {
				if (signal?.aborted) return
				setDiscoveredHosts([])
				setSelected(new Set())
				setFeedback(error instanceof Error ? error.message : t`Unable to read the SSH config.`)
			} finally {
				if (!signal?.aborted) setLoading(false)
			}
		},
		[t]
	)

	useEffect(() => {
		const controller = new AbortController()
		loadHosts($userSettings.get().sshConfigPath ?? "", controller.signal).catch(console.error)
		return () => controller.abort()
	}, [loadHosts])

	async function importSelectedHosts() {
		const userId = pb.authStore.record?.id
		const portNumber = Number(port)
		if (path.trim() !== loadedPath) {
			setFeedback(t`Reload the SSH config after changing its path.`)
			return
		}
		if (!userId || !Number.isInteger(portNumber) || portNumber < 1 || portNumber > 65535) {
			setFeedback(t`Enter a valid agent port between 1 and 65535.`)
			return
		}
		setImporting(true)
		setFeedback("")
		try {
			const results = await Promise.allSettled(
				actionTargets.importHosts.map((host) =>
					pb.collection("systems").create(
						{
							name: host.name,
							host: host.name,
							port,
							ssh_config: loadedPath,
							pkey: $publicKey.get(),
							users: userId,
							status: "pending",
						},
						{ requestKey: null }
					)
				)
			)
			let imported = 0
			const failedHosts: SSHHost[] = []
			for (const [index, result] of results.entries()) {
				if (result.status === "fulfilled") imported++
				else if (actionTargets.importHosts[index]) failedHosts.push(actionTargets.importHosts[index])
			}
			setFeedback(
				failedHosts.length ? t`${imported} imported, ${failedHosts.length} failed.` : t`${imported} systems imported.`
			)
			setSelected(new Set(failedHosts.map(({ name }) => name)))
		} catch (error: unknown) {
			setFeedback(error instanceof Error ? error.message : t`Unable to import the selected SSH hosts.`)
		} finally {
			setImporting(false)
		}
	}

	async function updateSelectedPaths() {
		setUpdating(true)
		setFeedback("")
		try {
			let batch = pb.createBatch()
			let queued = 0
			for (const system of updatePathSystems) {
				batch.collection("systems").update(system.id, { ssh_config: loadedPath, status: "pending" })
				queued++
				if (queued >= 20) {
					await batch.send()
					batch = pb.createBatch()
					queued = 0
				}
			}
			if (queued > 0) await batch.send()
			setFeedback(t`${updatePathSystems.length} SSH config paths updated.`)
		} catch (error: unknown) {
			setFeedback(error instanceof Error ? error.message : t`Unable to update the selected SSH config paths.`)
		} finally {
			setUpdating(false)
		}
	}

	const busy = loading || importing || updating
	return (
		<div className="grid min-h-0 min-w-0 gap-4">
			<div className="grid gap-2">
				<Label htmlFor="ssh-config-path">
					<Trans>SSH config path</Trans>
				</Label>
				<div className="flex gap-2">
					<Input
						id="ssh-config-path"
						className="font-mono min-w-0"
						value={path}
						onChange={(event) => setPath(event.target.value)}
						disabled={isReadOnlyUser()}
					/>
					<Button
						type="button"
						variant="outline"
						size="icon"
						onClick={() => loadHosts(path)}
						disabled={busy}
						aria-label={t`Reload SSH config`}
					>
						<RefreshCwIcon className={cn("size-4", loading && "animate-spin")} />
					</Button>
				</div>
				{isReadOnlyUser() && (
					<p className="text-xs text-muted-foreground">
						<Trans>Read-only users cannot change this path.</Trans>
					</p>
				)}
			</div>
			<SSHHostList
				hosts={hosts}
				systems={systems}
				selected={selected}
				loading={loading}
				onSelectionChange={setSelected}
			/>
			<div className="grid gap-2 sm:max-w-48">
				<Label htmlFor="ssh-import-port">
					<Trans>Beszel agent port</Trans>
				</Label>
				<Input
					id="ssh-import-port"
					type="number"
					min="1"
					max="65535"
					value={port}
					onChange={(event) => setPort(event.target.value)}
				/>
			</div>
			{feedback && (
				<output aria-live="polite" className="text-sm text-muted-foreground">
					{feedback}
				</output>
			)}
			<DialogFooter className="gap-2">
				<div className="grid gap-2 sm:flex sm:flex-wrap sm:justify-end">
					<Button
						type="button"
						variant="outline"
						onClick={updateSelectedPaths}
						disabled={busy || updatePathSystems.length === 0}
					>
						<SaveIcon className="me-2 size-4" />
						{updating ? <Trans>Updating...</Trans> : <Trans>Update path</Trans>} ({updatePathSystems.length})
					</Button>
					<Button type="button" onClick={importSelectedHosts} disabled={busy || actionTargets.importHosts.length === 0}>
						<UploadIcon className="me-2 size-4" />
						{importing ? <Trans>Importing...</Trans> : <Trans>Import selected</Trans>} (
						{actionTargets.importHosts.length})
					</Button>
				</div>
			</DialogFooter>
		</div>
	)
}
