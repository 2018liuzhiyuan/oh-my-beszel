import { describe, expect, test } from "bun:test"
import { filterSSHHosts, getSSHHostActionTargets } from "./ssh-host-actions"

describe("getSSHHostActionTargets", () => {
	test("separates new hosts from SSH-managed systems when names are selected", () => {
		// Given
		const hosts = [
			{ name: "gpu-new", hostName: "10.0.0.10" },
			{ name: "gpu-managed", hostName: "10.0.0.11" },
			{ name: "manual-system", hostName: "10.0.0.12" },
		]
		const systems = [
			{ id: "managed-id", name: "gpu-managed", host: "gpu-managed", ssh_config: "/srv/ssh/config" },
			{ id: "manual-id", name: "manual-system", host: "10.0.0.20", ssh_config: "" },
		]
		const selected = new Set(["gpu-new", "gpu-managed", "manual-system"])

		// When
		const targets = getSSHHostActionTargets(hosts, systems, selected)

		// Then: selected managed systems may update their config path, but deletion is not an SSH import action.
		expect(targets).toEqual({
			importHosts: [{ name: "gpu-new", hostName: "10.0.0.10" }],
			managedSystems: [{ id: "managed-id", name: "gpu-managed", host: "gpu-managed", ssh_config: "/srv/ssh/config" }],
		})
	})
})

describe("filterSSHHosts", () => {
	const hosts = [
		{ name: "gpu-alpha", hostName: "10.0.0.10" },
		{ name: "GPU-beta", hostName: "compute-beta.internal" },
		{ name: "meta-01", hostName: "10.240.16.7" },
	]

	test("matches aliases and resolved hosts by case-insensitive prefix", () => {
		expect(filterSSHHosts(hosts, "gpu-")).toEqual({ hosts: hosts.slice(0, 2), invalidRegex: false })
		expect(filterSSHHosts(hosts, "10.240")).toEqual({ hosts: [hosts[2]], invalidRegex: false })
	})

	test("matches slash-delimited regular expressions", () => {
		expect(filterSSHHosts(hosts, "/^(gpu|meta)-/")).toEqual({ hosts, invalidRegex: false })
	})

	test("reports invalid regular expressions", () => {
		expect(filterSSHHosts(hosts, "/[/")).toEqual({ hosts: [], invalidRegex: true })
	})

	test("limits selected import targets to visible matches and disables invalid searches", () => {
		const selected = new Set(hosts.map((host) => host.name))
		const prefix = filterSSHHosts(hosts, "gpu-")
		expect(getSSHHostActionTargets(prefix.hosts, [], selected).importHosts).toEqual(hosts.slice(0, 2))
		const invalid = filterSSHHosts(hosts, "/[/")
		expect(getSSHHostActionTargets(invalid.hosts, [], selected).importHosts).toEqual([])
		const noMatches = filterSSHHosts(hosts, "missing-")
		expect(getSSHHostActionTargets(noMatches.hosts, [], selected).importHosts).toEqual([])
	})
})
