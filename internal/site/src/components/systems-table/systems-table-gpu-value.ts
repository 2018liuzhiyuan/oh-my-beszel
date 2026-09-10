type GpuSummary = {
	readonly g?: number
	readonly gm?: number
}

export function getGpuUtilization({ g, gm }: GpuSummary): number | undefined {
	return g ?? (gm === undefined ? undefined : 0)
}
