import { Trans } from "@lingui/react/macro"
import { useStore } from "@nanostores/react"
import { getPagePath } from "@nanostores/router"
import { useEffect, useRef, useState } from "react"
import { SystemStatus } from "@/lib/enums"
import { pb } from "@/lib/api"
import { $publicKey } from "@/lib/stores"
import { cn, generateToken, tokenMap } from "@/lib/utils"
import type { SystemRecord } from "@/types"
import { $router, Link, navigate } from "./router"
import { type InstallTab, SystemInstallAction } from "./system-install-actions"
import { Button } from "./ui/button"
import { DialogFooter } from "./ui/dialog"
import { Input } from "./ui/input"
import { InputCopy } from "./ui/input-copy"
import { Label } from "./ui/label"

let nextSystemToken: string | null = null

type SystemAgentFormProps = {
	readonly tab: InstallTab
	readonly setOpen: (open: boolean) => void
	readonly system?: SystemRecord
}

export function SystemAgentForm({ tab, setOpen, system }: SystemAgentFormProps) {
	const publicKey = useStore($publicKey)
	const port = useRef<HTMLInputElement>(null)
	const [name, setName] = useState(system?.name ?? "")
	const [host, setHost] = useState(system?.host ?? "")
	const [token, setToken] = useState(system?.token ?? "")
	const isUnixSocket = host.startsWith("/")

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
			.getFirstListItem(`system = "${systemId}"`, { fields: "token" })
			.then(({ token: storedToken }) => {
				if (!active) return
				tokenMap.set(systemId, storedToken)
				setToken(storedToken)
			})
			.catch((error: unknown) => {
				if (active) console.error(error)
			})
		return () => {
			active = false
		}
	}, [system?.id])

	const getAgentPort = () => (isUnixSocket ? host : port.current?.value || system?.port || "45876")

	async function handleSubmit(event: React.FormEvent<HTMLFormElement>) {
		event.preventDefault()
		const userId = pb.authStore.record?.id
		if (!userId) return
		const data: Record<string, FormDataEntryValue | string> = Object.fromEntries(new FormData(event.currentTarget))
		data.users = userId
		try {
			if (system) {
				await pb.collection("systems").update(system.id, { ...data, status: SystemStatus.Pending })
			} else {
				const createdSystem = await pb.collection("systems").create(data)
				await pb.collection("fingerprints").create({ system: createdSystem.id, token })
				nextSystemToken = null
			}
			setOpen(false)
			navigate(getPagePath($router, "home"))
		} catch (error: unknown) {
			console.error(error)
		}
	}

	return (
		<>
			<p className="mb-3 min-w-full text-sm leading-relaxed text-muted-foreground">
				{tab === "docker" ? (
					<Trans>
						Copy the <code className="bg-muted px-1 rounded-sm leading-3">docker-compose.yml</code> content for the
						agent below, or register agents automatically with a <TokenSettingsLink setOpen={setOpen} />.
					</Trans>
				) : (
					<Trans>
						Copy the installation command for the agent below, or register agents automatically with a{" "}
						<TokenSettingsLink setOpen={setOpen} />.
					</Trans>
				)}
			</p>
			<form onSubmit={handleSubmit}>
				<div className="grid xs:grid-cols-[auto_1fr] gap-y-3 gap-x-4 items-center mb-4">
					<Label htmlFor="name" className="xs:text-end">
						<Trans>Name</Trans>
					</Label>
					<Input id="name" name="name" value={name} onChange={(event) => setName(event.target.value)} required />
					<Label htmlFor="host" className="xs:text-end">
						<Trans>Host / IP</Trans>
					</Label>
					<Input id="host" name="host" value={host} onChange={(event) => setHost(event.target.value)} required />
					<Label htmlFor="port" className={cn("xs:text-end", isUnixSocket && "hidden")}>
						<Trans>Port</Trans>
					</Label>
					<Input
						ref={port}
						id="port"
						name="port"
						defaultValue={system?.port || "45876"}
						required={!isUnixSocket}
						className={cn(isUnixSocket && "hidden")}
					/>
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
					<SystemInstallAction tab={tab} credentials={{ port: getAgentPort(), publicKey, token }} />
					<Button>{system ? <Trans>Save System</Trans> : <Trans>Add System</Trans>}</Button>
				</DialogFooter>
			</form>
		</>
	)
}

function TokenSettingsLink({ setOpen }: { readonly setOpen: (open: boolean) => void }) {
	return (
		<Link onClick={() => setOpen(false)} href={getPagePath($router, "settings", { name: "tokens" })} className="link">
			universal token
		</Link>
	)
}
