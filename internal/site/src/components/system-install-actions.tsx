import { t } from "@lingui/core/macro"
import { ChevronDownIcon, ExternalLinkIcon } from "lucide-react"
import { memo } from "react"
import {
	copyDockerCompose,
	copyDockerRun,
	copyLinuxCommand,
	copyWindowsCommand,
	type DropdownItem,
	InstallDropdown,
} from "./install-dropdowns"
import { Button } from "./ui/button"
import { DropdownMenu, DropdownMenuTrigger } from "./ui/dropdown-menu"
import { AppleIcon, DockerIcon, FreeBsdIcon, TuxIcon, WindowsIcon } from "./ui/icons"

export type InstallTab = "docker" | "binary"

type AgentCredentials = {
	readonly port: string
	readonly publicKey: string
	readonly token: string
}

type SystemInstallActionProps = {
	readonly tab: InstallTab
	readonly credentials: AgentCredentials
}

export function SystemInstallAction({ tab, credentials }: SystemInstallActionProps) {
	if (tab === "docker") {
		return (
			<CopyButton
				text={t({ message: "Copy docker compose", context: "Button to copy docker compose file content" })}
				onClick={async () => copyDockerCompose(credentials.port, credentials.publicKey, credentials.token)}
				icon={<DockerIcon className="size-4 -me-0.5" />}
				dropdownItems={[
					{
						text: t({ message: "Copy docker run", context: "Button to copy docker run command" }),
						onClick: async () => copyDockerRun(credentials.port, credentials.publicKey, credentials.token),
						icons: [DockerIcon],
					},
				]}
			/>
		)
	}

	return (
		<CopyButton
			text={t`Copy Linux command`}
			icon={<TuxIcon className="size-4" />}
			onClick={async () => copyLinuxCommand(credentials.port, credentials.publicKey, credentials.token)}
			dropdownItems={[
				{
					text: t({ message: "Homebrew command", context: "Button to copy install command" }),
					onClick: async () => copyLinuxCommand(credentials.port, credentials.publicKey, credentials.token, true),
					icons: [AppleIcon, TuxIcon],
				},
				{
					text: t({ message: "Windows command", context: "Button to copy install command" }),
					onClick: async () => copyWindowsCommand(credentials.port, credentials.publicKey, credentials.token),
					icons: [WindowsIcon],
				},
				{
					text: t({ message: "FreeBSD command", context: "Button to copy install command" }),
					onClick: async () => copyLinuxCommand(credentials.port, credentials.publicKey, credentials.token),
					icons: [FreeBsdIcon],
				},
				{
					text: t`Manual setup instructions`,
					url: "https://beszel.dev/guide/agent-installation#binary",
					icons: [ExternalLinkIcon],
				},
			]}
		/>
	)
}

type CopyButtonProps = {
	readonly text: string
	readonly onClick: () => void
	readonly dropdownItems: readonly DropdownItem[]
	readonly icon?: React.ReactNode
}

const CopyButton = memo(({ text, onClick, dropdownItems, icon }: CopyButtonProps) => (
	<div className="flex gap-0 rounded-lg">
		<Button type="button" variant="outline" onClick={onClick} className="rounded-e-none dark:border-e-0 grow gap-2">
			{text} {icon}
		</Button>
		<div className="w-px h-full bg-muted" />
		<DropdownMenu>
			<DropdownMenuTrigger asChild>
				<Button type="button" variant="outline" className="px-2 rounded-s-none border-s-0" aria-label={t`More options`}>
					<ChevronDownIcon />
				</Button>
			</DropdownMenuTrigger>
			<InstallDropdown items={[...dropdownItems]} />
		</DropdownMenu>
	</div>
))
