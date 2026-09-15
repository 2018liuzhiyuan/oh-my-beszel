import { Trans, useLingui } from "@lingui/react/macro"
import { Trash2Icon, XIcon } from "lucide-react"
import { useEffect, useState } from "react"
import { pb } from "@/lib/api"
import { cn } from "@/lib/utils"
import type { SystemRecord } from "@/types"
import {
	AlertDialog,
	AlertDialogAction,
	AlertDialogCancel,
	AlertDialogContent,
	AlertDialogDescription,
	AlertDialogFooter,
	AlertDialogHeader,
	AlertDialogTitle,
} from "../ui/alert-dialog"
import { Button, buttonVariants } from "../ui/button"

// mirrors the SSH host manager's batch size
const DELETE_BATCH_SIZE = 20

const PREVIEW_NAMES = 5

/**
 * Toolbar shown while systems are selected in the table. Deletes in batches so
 * a single failed batch leaves the remaining systems selected for a retry.
 */
export function BulkActionsBar({
	systems,
	onFinished,
}: {
	readonly systems: readonly SystemRecord[]
	readonly onFinished: (failedIds: readonly string[]) => void
}) {
	const { t } = useLingui()
	const [confirmOpen, setConfirmOpen] = useState(false)
	const [deleting, setDeleting] = useState(false)
	const [failedCount, setFailedCount] = useState(0)

	// Escape clears the selection, unless the confirm dialog is open (Radix
	// closes the dialog on Escape itself).
	useEffect(() => {
		if (confirmOpen) return
		const onKeyDown = (event: KeyboardEvent) => {
			if (event.key === "Escape") onFinished([])
		}
		window.addEventListener("keydown", onKeyDown)
		return () => window.removeEventListener("keydown", onKeyDown)
	}, [confirmOpen, onFinished])

	async function deleteSelected() {
		setDeleting(true)
		const failed: string[] = []
		for (let index = 0; index < systems.length; index += DELETE_BATCH_SIZE) {
			const slice = systems.slice(index, index + DELETE_BATCH_SIZE)
			const batch = pb.createBatch()
			for (const system of slice) {
				batch.collection("systems").delete(system.id)
			}
			try {
				await batch.send()
			} catch (error) {
				console.error("Bulk delete batch failed", error)
				failed.push(...slice.map(({ id }) => id))
			}
		}
		setDeleting(false)
		setConfirmOpen(false)
		setFailedCount(failed.length)
		onFinished(failed)
	}

	const names = systems.map(({ name }) => name)
	const preview = names.slice(0, PREVIEW_NAMES).join(", ")
	const extraCount = Math.max(0, names.length - PREVIEW_NAMES)

	return (
		<>
			<div className="mb-3 flex flex-wrap items-center gap-x-3 gap-y-1 rounded-md border bg-accent/40 px-3 py-2 text-sm">
				<span className="font-medium">
					<Trans>{systems.length} selected</Trans>
				</span>
				{failedCount > 0 && (
					<span className="text-destructive">
						<Trans>Failed to delete {failedCount} systems.</Trans>
					</span>
				)}
				<div className="ms-auto flex items-center gap-1.5">
					<Button
						type="button"
						variant="destructive"
						size="sm"
						disabled={deleting}
						onClick={() => setConfirmOpen(true)}
					>
						<Trash2Icon className="me-1.5 size-4" />
						{deleting ? <Trans>Deleting...</Trans> : <Trans>Delete selected</Trans>}
					</Button>
					<Button
						type="button"
						variant="ghost"
						size="icon"
						aria-label={t`Clear selection`}
						title={t`Clear selection`}
						disabled={deleting}
						onClick={() => onFinished([])}
					>
						<XIcon className="size-4" />
					</Button>
				</div>
			</div>
			<AlertDialog open={confirmOpen} onOpenChange={setConfirmOpen}>
				<AlertDialogContent>
					<AlertDialogHeader>
						<AlertDialogTitle>
							<Trans>Delete {systems.length} systems?</Trans>
						</AlertDialogTitle>
						<AlertDialogDescription>
							<Trans>
								This action cannot be undone. All current records for{" "}
								{extraCount > 0 ? `${preview} (+${extraCount})` : preview} will be permanently deleted from the
								database.
							</Trans>
						</AlertDialogDescription>
					</AlertDialogHeader>
					<AlertDialogFooter>
						<AlertDialogCancel>
							<Trans>Cancel</Trans>
						</AlertDialogCancel>
						<AlertDialogAction
							className={cn(buttonVariants({ variant: "destructive" }))}
							disabled={deleting}
							onClick={(event) => {
								// keep the dialog open until deletion finishes
								event.preventDefault()
								deleteSelected().catch(console.error)
							}}
						>
							{deleting ? <Trans>Deleting...</Trans> : <Trans>Delete {systems.length} systems</Trans>}
						</AlertDialogAction>
					</AlertDialogFooter>
				</AlertDialogContent>
			</AlertDialog>
		</>
	)
}
