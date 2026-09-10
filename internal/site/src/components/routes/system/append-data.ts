export function appendData<T extends { created: string | number | null }>(
	previous: T[],
	newRecords: T[],
	maxLength?: number
): T[] {
	if (!newRecords.length) return previous
	const trimmed =
		maxLength && previous.length >= maxLength ? previous.slice(-(maxLength - newRecords.length)) : previous
	const result = trimmed.slice()
	for (const record of newRecords) {
		if (record.created !== null) {
			if (typeof record.created === "string") {
				record.created = new Date(record.created).getTime()
			}
		}
		result.push(record)
	}
	return result
}
