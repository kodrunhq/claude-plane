import { NavLink } from 'react-router';
import { LayoutDashboard, Terminal, Server, Settings } from 'lucide-react';
import { useUIStore } from '../../stores/ui.ts';

const navItems = [
  { to: '/', label: 'Command Center', icon: LayoutDashboard },
  { to: '/sessions', label: 'Sessions', icon: Terminal },
  { to: '/machines', label: 'Machines', icon: Server },
] as const;

export function Sidebar() {
  const collapsed = useUIStore((s) => s.sidebarCollapsed);

  return (
    <aside
      className="flex flex-col bg-bg-secondary border-r border-gray-700 transition-all duration-200"
      style={{ width: collapsed ? 64 : 240 }}
    >
      <nav className="flex-1 flex flex-col gap-1 p-2 mt-2">
        {navItems.map(({ to, label, icon: Icon }) => (
          <NavLink
            key={to}
            to={to}
            end={to === '/'}
            className={({ isActive }) =>
              `flex items-center gap-3 px-3 py-2 rounded-md text-sm transition-colors ${
                isActive
                  ? 'bg-bg-tertiary text-accent-primary'
                  : 'text-text-secondary hover:text-text-primary hover:bg-bg-tertiary/50'
              }`
            }
          >
            <Icon size={20} className="shrink-0" />
            {!collapsed && <span>{label}</span>}
          </NavLink>
        ))}
      </nav>

      <div className="p-2 border-t border-gray-700">
        <button
          className="flex items-center gap-3 px-3 py-2 rounded-md text-sm text-text-secondary hover:text-text-primary hover:bg-bg-tertiary/50 w-full transition-colors"
          aria-label="Settings"
        >
          <Settings size={20} className="shrink-0" />
          {!collapsed && <span>Settings</span>}
        </button>
      </div>
    </aside>
  );
}
