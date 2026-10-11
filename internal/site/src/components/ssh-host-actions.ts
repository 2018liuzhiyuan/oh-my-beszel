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

export type SSHHostFilterResult = {
	readonly hosts: readonly SSHHost[]
	readonly invalidRegex: boolean
}

export function filterSSHHosts(hosts: readonly SSHHost[], query: string): SSHHostFilterResult {
	const value = query.trim()
	if (!value) return { hosts, invalidRegex: false }

	if (value.startsWith("/")) {
		if (value.length < 2 || !value.endsWith("/")) return { hosts: [], invalidRegex: true }
		try {
			const expression = new RegExp(value.slice(1, -1), "i")
			return {
				hosts: hosts.filter((host) => expression.test(host.name) || expression.test(host.hostName)),
				invalidRegex: false,
			}
		} catch {
			return { hosts: [], invalidRegex: true }
		}
	}

	const prefix = value.toLowerCase()
	return {
		hosts: hosts.filter(
			(host) => host.name.toLowerCase().startsWith(prefix) || host.hostName.toLowerCase().startsWith(prefix)
		),
		invalidRegex: false,
	}
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
