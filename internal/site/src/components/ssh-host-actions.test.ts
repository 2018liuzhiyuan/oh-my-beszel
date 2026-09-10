import { describe, expect, test } from "bun:test"
import { getSSHHostActionTargets } from "./ssh-host-actions"

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
