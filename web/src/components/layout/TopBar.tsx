import { Menu } from 'lucide-react';
import { useUIStore } from '../../stores/ui.ts';

export function TopBar() {
  const toggleSidebar = useUIStore((s) => s.toggleSidebar);

  return (
    <header className="flex items-center justify-between h-12 px-4 bg-bg-secondary border-b border-gray-700 shrink-0">
      <button
        onClick={toggleSidebar}
        className="p-1.5 rounded-md text-text-secondary hover:text-text-primary hover:bg-bg-tertiary transition-colors"
        aria-label="Toggle sidebar"
      >
        <Menu size={20} />
      </button>

      <span className="text-sm font-semibold text-text-primary tracking-wide font-mono">
        claude-plane
      </span>

      <div
        className="w-8 h-8 rounded-full bg-bg-tertiary flex items-center justify-center text-xs text-text-secondary font-medium"
        aria-label="User avatar"
      >
        CP
      </div>
    </header>
  );
}
