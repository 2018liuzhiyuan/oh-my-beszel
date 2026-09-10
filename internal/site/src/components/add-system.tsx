import { t } from "@lingui/core/macro"
import { Trans } from "@lingui/react/macro"
import { useStore } from "@nanostores/react"
import { getPagePath } from "@nanostores/router"
import { ChevronDownIcon, ExternalLinkIcon, RefreshCwIcon } from "lucide-react"
import { memo, useCallback, useEffect, useRef, useState } from "react"
import { Button } from "@/components/ui/button"
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { isReadOnlyUser, pb } from "@/lib/api"
import { SystemStatus } from "@/lib/enums"
import { $publicKey } from "@/lib/stores"
import { cn, generateToken, tokenMap, useBrowserStorage } from "@/lib/utils"
import type { SystemRecord } from "@/types"
import {
	copyDockerCompose,
	copyDockerRun,
	copyLinuxCommand,
	copyWindowsCommand,
	type DropdownItem,
	InstallDropdown,
} from "./install-dropdowns"
import { $router, Link, navigate } from "./router"
import { DropdownMenu, DropdownMenuTrigger } from "./ui/dropdown-menu"
import { AppleIcon, DockerIcon, FreeBsdIcon, TuxIcon, WindowsIcon } from "./ui/icons"
import { InputCopy } from "./ui/input-copy"
import { NewSystemDialog } from "./new-system-dialog"

export function AddSystemDialog({ open, setOpen }: { open: boolean; setOpen: (open: boolean) => void }) {
	if (isReadOnlyUser()) {
		return null
	}

	return (
		<Dialog open={open} onOpenChange={setOpen}>
			<NewSystemDialog setOpen={setOpen} />
		</Dialog>
	)
}

/**
 * Token to be used for the next system.
 * Prevents token changing if user copies config, then closes dialog and opens again.
 */
let nextSystemToken: string | null = null

type SSHHost = {
	name: string
	hostName: string
}

/**
 * SystemDialog component for adding or editing a system.
 * @param {Object} props - The component props.
 * @param {function} props.setOpen - Function to set the open state of the dialog.
 * @param {SystemRecord} [props.system] - Optional system record for editing an existing system.
 */
export const SystemDialog = ({
	setOpen,
	system,
	open,
}: {
	setOpen: (open: boolean) => void
	system?: SystemRecord
	open?: boolean
}) => {
	const publicKey = useStore($publicKey)
	const port = useRef<HTMLInputElement>(null)
	const [nameValue, setNameValue] = useState(system?.name ?? "")
	const [hostValue, setHostValue] = useState(system?.host ?? "")
	const isUnixSocket = hostValue.startsWith("/")
	const [tab, setTab] = useBrowserStorage("as-tab", "docker")
	const [token, setToken] = useState(system?.token ?? "")
	const [quickMode, setQuickMode] = useState(!system)
	const [sshHosts, setSSHHosts] = useState<SSHHost[]>([])
	const [sshHostValue, setSSHHostValue] = useState("")
	const [sshHostsLoading, setSSHHostsLoading] = useState(false)
	const [sshHostsError, setSSHHostsError] = useState(false)
	const selectedSSHHost = sshHosts.find((host) => host.name.toLowerCase() === sshHostValue.trim().toLowerCase())
	const sshHostNotFound = sshHostValue.trim() !== "" && !selectedSSHHost && !sshHostsLoading

	const loadSSHHosts = useCallback(async (signal?: AbortSignal) => {
		setSSHHostsLoading(true)
		setSSHHostsError(false)
		try {
			const response = await pb.send<{ hosts: SSHHost[] }>("/api/beszel/ssh-hosts", { signal })
			if (signal?.aborted) return
			setSSHHosts(response.hosts)
			setSSHHostsError(response.hosts.length === 0)
		} catch (error) {
			if (signal?.aborted) return
			console.error(error)
			setSSHHosts([])
			setSSHHostsError(true)
		} finally {
			if (!signal?.aborted) setSSHHostsLoading(false)
		}
	}, [])

	useEffect(() => {
		if (!system && open) {
			const controller = new AbortController()
			loadSSHHosts(controller.signal).catch(console.error)
			return () => controller.abort()
		}
	}, [loadSSHHosts, open, system])

	useEffect(() => {
		if (!quickMode) {
			return
		}
		setNameValue(selectedSSHHost?.name ?? "")
		setHostValue(selectedSSHHost?.hostName ?? "")
	}, [quickMode, selectedSSHHost])

	useEffect(() => {
		const systemId = system?.id
		if (!systemId) {
			nextSystemToken ||= generateToken()
			setToken(nextSystemToken)
			return
		}
		const cachedToken = tokenMap.get(systemId)
		if (cachedToken !== undefined) {
			setToken(cachedToken)
			return
		}
		let active = true
		pb.collection("fingerprints")
			.getFirstListItem(`system = "${systemId}"`, {
				fields: "token",
			})
			.then(({ token }) => {
				if (!active) return
				tokenMap.set(systemId, token)
				setToken(token)
			})
			.catch((error: unknown) => {
				if (active) console.error(error)
			})
		return () => {
			active = false
		}
	}, [system?.id])

	async function handleSubmit(e: React.FormEvent<HTMLFormElement>) {
		e.preventDefault()
		const userId = pb.authStore.record?.id
		if (!userId) {
			return
		}
		const formData = new FormData(e.currentTarget)
		const data: Record<string, FormDataEntryValue | string> = Object.fromEntries(formData)
		data.users = userId
		try {
			setOpen(false)
			if (system) {
				await pb.collection("systems").update(system.id, { ...data, status: SystemStatus.Pending })
			} else {
				const createdSystem = await pb.collection("systems").create(data)
				await pb.collection("fingerprints").create({
					system: createdSystem.id,
					token,
				})
				// Reset the current token after successful system
				// creation so next system gets a new token
				nextSystemToken = null
			}
			navigate(getPagePath($router, "home"))
		} catch (e) {
			console.error(e)
		}
	}

	const getAgentPort = () => (isUnixSocket ? hostValue : port.current?.value || system?.port || "45876")

	const systemTranslation = t`System`

	return (
		<DialogContent
			className="w-[90%] sm:w-auto sm:ns-dialog max-w-full rounded-lg"
			onCloseAutoFocus={() => {
				setNameValue(system?.name ?? "")
				setHostValue(system?.host ?? "")
				setSSHHostValue("")
				setQuickMode(!system)
			}}
		>
			<Tabs defaultValue={tab} onValueChange={setTab}>
				<DialogHeader>
					<DialogTitle className="mb-1 pb-1 max-w-100 truncate pr-8">
						{system ? (
							<Trans>Edit {{ foo: systemTranslation }}</Trans>
						) : (
							<Trans>Add {{ foo: systemTranslation }}</Trans>
						)}
					</DialogTitle>
					<TabsList className="grid w-full grid-cols-2">
						<TabsTrigger value="docker">Docker</TabsTrigger>
						<TabsTrigger value="binary">
							<Trans>Binary</Trans>
						</TabsTrigger>
					</TabsList>
				</DialogHeader>
				{quickMode && (
					<DialogDescription className="sr-only">
						<Trans>Select an SSH Host from your local SSH config.</Trans>
					</DialogDescription>
				)}
				{/* Docker (set tab index to prevent auto focusing content in edit system dialog) */}
				<TabsContent value="docker" tabIndex={-1}>
					{!quickMode && (
						<DialogDescription className="mb-3 leading-relaxed w-0 min-w-full">
							<Trans>
								Copy the
								<code className="bg-muted px-1 rounded-sm leading-3">docker-compose.yml</code> content for the agent
								below, or register agents automatically with a{" "}
								<Link
									onClick={() => setOpen(false)}
									href={getPagePath($router, "settings", { name: "tokens" })}
									className="link"
								>
									universal token
								</Link>
								.
							</Trans>
						</DialogDescription>
					)}
				</TabsContent>
				{/* Binary */}
				<TabsContent value="binary" tabIndex={-1}>
					{!quickMode && (
						<DialogDescription className="mb-3 leading-relaxed w-0 min-w-full">
							<Trans>
								Copy the installation command for the agent below, or register agents automatically with a{" "}
								<Link
									onClick={() => setOpen(false)}
									href={getPagePath($router, "settings", { name: "tokens" })}
									className="link"
								>
									universal token
								</Link>
								.
							</Trans>
						</DialogDescription>
					)}
				</TabsContent>
				<form onSubmit={handleSubmit}>
					{!system && (
						<div className="flex justify-end -mb-1">
							<Button type="button" variant="link" size="sm" onClick={() => setQuickMode((value) => !value)}>
								{quickMode ? <Trans>Manual setup instructions</Trans> : "SSH config"}
							</Button>
						</div>
					)}
					<div className="grid xs:grid-cols-[auto_1fr] gap-y-3 gap-x-4 items-center mt-1 mb-4">
						{quickMode ? (
							<>
								<Label htmlFor="ssh-host" className="xs:text-end">
									SSH Host
								</Label>
								<div className="space-y-1.5 min-w-0">
									<div className="flex gap-2">
										<Input
											id="ssh-host"
											list="ssh-host-options"
											value={sshHostValue}
											onChange={(event) => setSSHHostValue(event.target.value)}
											placeholder="gpu-node-a"
											autoComplete="off"
											aria-invalid={sshHostNotFound}
											aria-describedby="ssh-host-status"
											className={cn(sshHostNotFound && "border-destructive focus-visible:ring-destructive")}
											required
										/>
										<datalist id="ssh-host-options">
											{sshHosts.map((host) => (
												<option key={host.name} value={host.name}>
													{host.hostName}
												</option>
											))}
										</datalist>
										<Button
											type="button"
											variant="outline"
											size="icon"
								onClick={() => loadSSHHosts()}
											disabled={sshHostsLoading}
											aria-label={t`Refresh`}
											title={t`Refresh`}
										>
											<RefreshCwIcon className={cn("size-4", sshHostsLoading && "animate-spin")} />
										</Button>
									</div>
									<p
										id="ssh-host-status"
										aria-live="polite"
										className={cn(
											"text-xs truncate",
											sshHostsError || sshHostNotFound ? "text-destructive" : "text-muted-foreground"
										)}
									>
										{selectedSSHHost
											? `${selectedSSHHost.name} → ${selectedSSHHost.hostName}:45876`
											: sshHostsLoading
												? t`Loading...`
												: sshHostNotFound
													? t`SSH Host not found in ~/.ssh/config.`
													: sshHostsError
														? t`No results found.`
														: `~/.ssh/config · ${sshHosts.length}`}
									</p>
									<input type="hidden" name="name" value={nameValue} />
									<input type="hidden" name="host" value={hostValue} />
									<input type="hidden" name="port" value="45876" />
								</div>
							</>
						) : (
							<>
								<Label htmlFor="name" className="xs:text-end">
									<Trans>Name</Trans>
								</Label>
								<Input
									id="name"
									name="name"
									value={nameValue}
									onChange={(event) => setNameValue(event.target.value)}
									required
								/>
								<Label htmlFor="host" className="xs:text-end">
									<Trans>Host / IP</Trans>
								</Label>
								<Input
									id="host"
									name="host"
									value={hostValue}
									required
									onChange={(event) => setHostValue(event.target.value)}
								/>
								<Label htmlFor="port" className={cn("xs:text-end", isUnixSocket && "hidden")}>
									<Trans>Port</Trans>
								</Label>
								<Input
									ref={port}
									name="port"
									id="port"
									defaultValue={system?.port || "45876"}
									required={!isUnixSocket}
									className={cn(isUnixSocket && "hidden")}
								/>
							</>
						)}
						<Label htmlFor="pkey" className="xs:text-end whitespace-pre">
							<Trans comment="Use 'Key' if your language requires many more characters">Public Key</Trans>
						</Label>
						<InputCopy value={publicKey} id="pkey" name="pkey" />
						<Label htmlFor="tkn" className="xs:text-end whitespace-pre">
							<Trans>Token</Trans>
						</Label>
						<InputCopy value={token} id="tkn" name="tkn" />
					</div>
					<DialogFooter className="flex justify-end gap-x-2 gap-y-3 flex-col mt-5">
						{/* Docker */}
						<TabsContent value="docker" className="contents">
							<CopyButton
								text={t({ message: "Copy docker compose", context: "Button to copy docker compose file content" })}
								onClick={async () => copyDockerCompose(getAgentPort(), publicKey, token)}
								icon={<DockerIcon className="size-4 -me-0.5" />}
								dropdownItems={[
									{
										text: t({ message: "Copy docker run", context: "Button to copy docker run command" }),
										onClick: async () => copyDockerRun(getAgentPort(), publicKey, token),
										icons: [DockerIcon],
									},
								]}
							/>
						</TabsContent>
						{/* Binary */}
						<TabsContent value="binary" className="contents">
							<CopyButton
								text={t`Copy Linux command`}
								icon={<TuxIcon className="size-4" />}
								onClick={async () => copyLinuxCommand(getAgentPort(), publicKey, token)}
								dropdownItems={[
									{
										text: t({ message: "Homebrew command", context: "Button to copy install command" }),
										onClick: async () => copyLinuxCommand(getAgentPort(), publicKey, token, true),
										icons: [AppleIcon, TuxIcon],
									},
									{
										text: t({ message: "Windows command", context: "Button to copy install command" }),
										onClick: async () => copyWindowsCommand(getAgentPort(), publicKey, token),
										icons: [WindowsIcon],
									},
									{
										text: t({ message: "FreeBSD command", context: "Button to copy install command" }),
										onClick: async () => copyLinuxCommand(getAgentPort(), publicKey, token),
										icons: [FreeBsdIcon],
									},
									{
										text: t`Manual setup instructions`,
										url: "https://beszel.dev/guide/agent-installation#binary",
										icons: [ExternalLinkIcon],
									},
								]}
							/>
						</TabsContent>
						{/* Save */}
						<Button disabled={quickMode && !selectedSSHHost}>
							{system ? (
								<Trans>Save {{ foo: systemTranslation }}</Trans>
							) : (
								<Trans>Add {{ foo: systemTranslation }}</Trans>
							)}
						</Button>
					</DialogFooter>
				</form>
			</Tabs>
		</DialogContent>
	)
}

interface CopyButtonProps {
	text: string
	onClick: () => void
	dropdownItems: DropdownItem[]
	icon?: React.ReactNode
}

const CopyButton = memo((props: CopyButtonProps) => {
	return (
		<div className="flex gap-0 rounded-lg">
			<Button
				type="button"
				variant="outline"
				onClick={props.onClick}
				className="rounded-e-none dark:border-e-0 grow flex items-center gap-2"
			>
				{props.text} {props.icon}
			</Button>
			<div className="w-px h-full bg-muted"></div>
			<DropdownMenu>
				<DropdownMenuTrigger asChild>
					<Button variant="outline" className={"px-2 rounded-s-none border-s-0"}>
						<ChevronDownIcon />
					</Button>
				</DropdownMenuTrigger>
				<InstallDropdown items={props.dropdownItems} />
			</DropdownMenu>
		</div>
	)
})
