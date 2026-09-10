import { timeTicks } from "d3-time"
import { getPbTimestamp, pb } from "@/lib/api"
import { chartTimeData } from "@/lib/utils"
import type { ChartData, ChartTimes, ContainerStatsRecord, SystemStatsRecord } from "@/types"
export { appendData } from "./append-data"

type ChartTimeData = {
	time: number
	data: {
		ticks: number[]
		domain: number[]
	}
	chartTime: ChartTimes
}

export const cache = new Map<
	string,
	ChartTimeData | SystemStatsRecord[] | ContainerStatsRecord[] | ChartData["containerData"]
>()

// create ticks and domain for charts
export function getTimeData(chartTime: ChartTimes, lastCreated: number) {
	const cached = cache.get("td") as ChartTimeData | undefined
	if (cached && cached.chartTime === chartTime) {
		if (!lastCreated || cached.time >= lastCreated) {
			return cached.data
		}
	}

	// const buffer = chartTime === "1m" ? 400 : 20_000
	const now = new Date(Date.now())
	const startTime = chartTimeData[chartTime].getOffset(now)
	const ticks = timeTicks(startTime, now, chartTimeData[chartTime].ticks ?? 12).map((date) => date.getTime())
	const data = {
		ticks,
		domain: [chartTimeData[chartTime].getOffset(now).getTime(), now.getTime()],
	}
	cache.set("td", { time: now.getTime(), data, chartTime })
	return data
}

export async function getStats<T extends SystemStatsRecord | ContainerStatsRecord>(
	collection: string,
	systemId: string,
	chartTime: ChartTimes
): Promise<T[]> {
	const cachedStats = cache.get(`${systemId}_${chartTime}_${collection}`) as T[] | undefined
	const lastCached = cachedStats?.at(-1)?.created as number
	return await pb.collection<T>(collection).getFullList({
		filter: pb.filter("system={:id} && created > {:created} && type={:type}", {
			id: systemId,
			created: getPbTimestamp(chartTime, lastCached ? new Date(lastCached + 1000) : undefined),
			type: chartTimeData[chartTime].type,
		}),
		fields: "created,stats",
		sort: "created",
	})
}

export function makeContainerData(containers: ContainerStatsRecord[]): ChartData["containerData"] {
	const result = [] as ChartData["containerData"]
	for (const { created, stats } of containers) {
		if (!created) {
			result.push({ created: null } as ChartData["containerData"][0])
			continue
		}
		result.push(makeContainerPoint(new Date(created).getTime(), stats))
	}
	return result
}

/** Transform a single realtime container stats message into a ChartDataContainer point. */
export function makeContainerPoint(
	created: number,
	stats: ContainerStatsRecord["stats"]
): ChartData["containerData"][0] {
	const point: ChartData["containerData"][0] = { created } as ChartData["containerData"][0]
	for (const container of stats) {
		;(point as Record<string, unknown>)[container.n] = container
	}
	return point
}

export function dockerOrPodman(str: string, isPodman: boolean): string {
	if (isPodman) {
		return str.replace("docker", "podman").replace("Docker", "Podman")
	}
	return str
}
