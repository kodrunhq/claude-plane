import { Menu } from 'lucide-react';
import { useUIStore } from '../../stores/ui.ts';
import { useAuthStore } from '../../stores/auth.ts';
import { useIsMobile } from '../../hooks/useMediaQuery.ts';
import { NotificationBadge } from '../shared/NotificationBadge.tsx';

function userInitials(displayName: string, email: string): string {
  const trimmed = displayName.trim();
  if (trimmed) {
    const parts = trimmed.split(/\s+/);
    if (parts.length >= 2) {
      return (parts[0][0] + parts[parts.length - 1][0]).toUpperCase();
    }
    return parts[0][0].toUpperCase();
  }
  if (email) {
    return email[0].toUpperCase();
  }
  return 'U';
}

export function TopBar() {
  const toggleSidebar = useUIStore((s) => s.toggleSidebar);
  const setSidebarOpen = useUIStore((s) => s.setSidebarOpen);
  const sidebarOpen = useUIStore((s) => s.sidebarOpen);
  const isMobile = useIsMobile();
  const user = useAuthStore((s) => s.user);

  const initials = user ? userInitials(user.displayName, user.email) : 'U';

  function handleMenuClick() {
    if (isMobile) {
      setSidebarOpen(!sidebarOpen);
    } else {
      toggleSidebar();
    }
  }

  return (
    <header className="flex items-center justify-between h-12 px-4 bg-bg-secondary border-b border-border-primary shrink-0">
      <button
        onClick={handleMenuClick}
        className="p-2 md:p-1.5 rounded-md text-text-secondary hover:text-text-primary hover:bg-bg-tertiary transition-colors"
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

      <div className="flex items-center gap-2">
        <kbd className="hidden sm:inline-flex items-center gap-0.5 px-1.5 py-0.5 text-[10px] font-mono text-text-secondary bg-bg-tertiary border border-border-primary rounded">
          <span className="text-xs">&#8984;</span>K
        </kbd>
        <NotificationBadge />
        <div
          className="w-8 h-8 rounded-full bg-gradient-to-br from-accent-primary to-accent-purple flex items-center justify-center text-xs text-white font-medium"
        aria-label="User avatar"
        title={user?.displayName || user?.email || 'User'}
      >
          {initials}
        </div>
      </div>
    </header>
  );
}
