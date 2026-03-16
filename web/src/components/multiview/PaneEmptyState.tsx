interface PaneEmptyStateProps {
  readonly message?: string;
  readonly onPickSession: () => void;
}

export function PaneEmptyState({ message, onPickSession }: PaneEmptyStateProps) {
  return (
    <div className="flex flex-col items-center justify-center h-full bg-bg-primary text-text-secondary">
      <p className="text-sm mb-3">{message ?? 'No session selected'}</p>
      <button
        onClick={onPickSession}
        className="px-3 py-1.5 text-sm rounded bg-accent-primary text-white hover:bg-accent-primary/80 transition-colors"
      >
        Pick a session
      </button>
    </div>
  );
}
