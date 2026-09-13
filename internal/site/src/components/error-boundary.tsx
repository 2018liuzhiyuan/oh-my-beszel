import { t } from "@lingui/core/macro"
import { Component, type ErrorInfo, type ReactNode } from "react"

interface ErrorBoundaryProps {
	children: ReactNode
}

interface ErrorBoundaryState {
	error: Error | null
}

/**
 * AppFallback / ErrorBoundary keeps a render crash from becoming a blank
 * page: users get an explanation, the error text, and a reload action.
 */
export class ErrorBoundary extends Component<ErrorBoundaryProps, ErrorBoundaryState> {
	state: ErrorBoundaryState = { error: null }

	static getDerivedStateFromError(error: Error): ErrorBoundaryState {
		return { error }
	}

	componentDidCatch(error: Error, info: ErrorInfo) {
		console.error("Unhandled render error", error, info.componentStack)
	}

	render() {
		if (!this.state.error) return this.props.children
		return <AppFallback error={this.state.error} />
	}
}

function AppFallback({ error }: { error: Error }) {
	return (
		<div className="grid min-h-96 place-items-center p-6">
			<div className="w-full max-w-lg rounded-lg border border-border bg-card p-6 text-center">
				<h2 className="mb-2 text-lg font-semibold">{t`Something went wrong`}</h2>
				<p className="mb-4 text-sm text-muted-foreground">
					{t`An unexpected error interrupted the page. Reloading usually fixes it.`}
				</p>
				<pre className="mb-4 max-h-48 overflow-auto rounded-md bg-muted p-3 text-left text-xs text-muted-foreground">
					{error.message}
				</pre>
				<button
					type="button"
					onClick={() => window.location.reload()}
					className="rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground"
				>
					{t`Reload page`}
				</button>
			</div>
		</div>
	)
}
