import { Trans } from "@lingui/react/macro"
import { useState } from "react"
import { SSHHostManager } from "./ssh-host-manager"
import { SystemAgentForm } from "./system-agent-form"
import type { InstallTab } from "./system-install-actions"
import { DialogContent, DialogDescription, DialogHeader, DialogTitle } from "./ui/dialog"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "./ui/tabs"

type NewSystemTab = InstallTab | "ssh"

type NewSystemDialogProps = {
	readonly setOpen: (open: boolean) => void
}

export function NewSystemDialog({ setOpen }: NewSystemDialogProps) {
	const [tab, setTab] = useState<NewSystemTab>("docker")

	function selectTab(value: string) {
		if (value === "docker" || value === "binary" || value === "ssh") setTab(value)
	}

	const installTab: InstallTab = tab === "binary" ? "binary" : "docker"
	return (
		<DialogContent className="w-[calc(100%-2rem)] max-w-2xl min-w-0 max-h-[calc(100dvh-2rem)] overflow-x-hidden overflow-y-auto rounded-lg">
			<Tabs value={tab} onValueChange={selectTab} className="min-w-0">
				<DialogHeader>
					<DialogTitle>
						<Trans>Add System</Trans>
					</DialogTitle>
					<DialogDescription className="sr-only">
						<Trans>Choose how to add or connect a Beszel system.</Trans>
					</DialogDescription>
					<TabsList className="grid h-11 w-full grid-cols-3 sm:h-10">
						<TabsTrigger value="docker">Docker</TabsTrigger>
						<TabsTrigger value="binary">
							<Trans>Binary</Trans>
						</TabsTrigger>
						<TabsTrigger value="ssh">SSH</TabsTrigger>
					</TabsList>
				</DialogHeader>
				<div className={tab === "ssh" ? "hidden" : "mt-2"}>
					<SystemAgentForm tab={installTab} setOpen={setOpen} />
				</div>
				<TabsContent value="ssh" tabIndex={-1} className="mt-3">
					<SSHHostManager />
				</TabsContent>
			</Tabs>
		</DialogContent>
	)
}
