import { Menu } from 'lucide-react';
import { useUIStore } from '../../stores/ui.ts';

export function TopBar() {
  const toggleSidebar = useUIStore((s) => s.toggleSidebar);

  return (
    <header className="flex items-center justify-between h-12 px-4 bg-bg-secondary border-b border-border-primary shrink-0">
      <button
        onClick={toggleSidebar}
        className="p-1.5 rounded-md text-text-secondary hover:text-text-primary hover:bg-bg-tertiary transition-colors"
        aria-label="Toggle sidebar"
      >
        <Menu size={20} />
      </button>

      <div className="flex items-center gap-2">
        <div className="w-2 h-2 rounded-full bg-accent-green animate-pulse" />
        <span
          className="text-sm font-semibold tracking-wide font-mono"
          style={{
            background: 'linear-gradient(135deg, #3b82f6, #06b6d4, #a855f7)',
            WebkitBackgroundClip: 'text',
            WebkitTextFillColor: 'transparent',
          }}
        >
          claude-plane
        </span>
      </div>

      <div
        className="w-8 h-8 rounded-full bg-gradient-to-br from-accent-primary to-accent-purple flex items-center justify-center text-xs text-white font-medium"
        aria-label="User avatar"
      >
        CP
      </div>
    </header>
  );
}
