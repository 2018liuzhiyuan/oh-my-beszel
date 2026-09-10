export type SSHHost = {
	readonly name: string
	readonly hostName: string
}

export type SSHManagedSystem = {
	readonly id: string
	readonly name: string
	readonly host: string
	readonly ssh_config: string
}

export type SSHHostActionTargets = {
	readonly importHosts: readonly SSHHost[]
	readonly managedSystems: readonly SSHManagedSystem[]
}

export function getSSHHostActionTargets(
	hosts: readonly SSHHost[],
	systems: readonly SSHManagedSystem[],
	selectedNames: ReadonlySet<string>
): SSHHostActionTargets {
	const systemsByName = new Map(systems.map((system) => [system.name.toLowerCase(), system]))
	const importHosts: SSHHost[] = []
	const managedSystems: SSHManagedSystem[] = []

	for (const host of hosts) {
		if (!selectedNames.has(host.name)) {
			continue
		}
		const system = systemsByName.get(host.name.toLowerCase())
		if (!system) {
			importHosts.push(host)
		} else if (system.ssh_config) {
			managedSystems.push(system)
		}
	}

	return { importHosts, managedSystems }
}
