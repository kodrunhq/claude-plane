export function StatusBar() {
  return (
    <footer className="flex items-center justify-between h-6 px-4 bg-bg-secondary border-t border-gray-700 text-xs text-text-secondary shrink-0">
      <div className="flex items-center gap-4">
        <span className="flex items-center gap-1.5">
          <span className="w-2 h-2 rounded-full bg-status-success" />
          Connected
        </span>
      </div>

      <div className="flex items-center gap-4">
        <span>0 machines</span>
        <span>0 sessions</span>
      </div>
    </footer>
  );
}
