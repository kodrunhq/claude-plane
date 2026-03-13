import { NavLink } from 'react-router';
import {
  LayoutDashboard,
  Terminal,
  Server,
  Workflow,
  Play,
  LogOut,
  Webhook,
  Clock,
  Activity,
  Users,
  KeyRound,
} from 'lucide-react';
import { useUIStore } from '../../stores/ui.ts';
import { useAuthStore } from '../../stores/auth.ts';

interface NavItem {
  to: string;
  label: string;
  icon: React.ComponentType<{ size?: number; className?: string }>;
}

interface NavSection {
  title: string;
  items: NavItem[];
  adminOnly?: boolean;
}

const navSections: NavSection[] = [
  {
    title: 'Core',
    items: [
      { to: '/', label: 'Command Center', icon: LayoutDashboard },
      { to: '/sessions', label: 'Sessions', icon: Terminal },
      { to: '/machines', label: 'Machines', icon: Server },
      { to: '/jobs', label: 'Jobs', icon: Workflow },
      { to: '/runs', label: 'Runs', icon: Play },
    ],
  },
  {
    title: 'Automation',
    items: [
      { to: '/webhooks', label: 'Webhooks', icon: Webhook },
      { to: '/schedules', label: 'Schedules', icon: Clock },
    ],
  },
  {
    title: 'Monitoring',
    items: [
      { to: '/events', label: 'Events', icon: Activity },
    ],
  },
  {
    title: 'Admin',
    adminOnly: true,
    items: [
      { to: '/users', label: 'Users', icon: Users },
      { to: '/provisioning', label: 'Provisioning', icon: KeyRound },
    ],
  },
];

function NavItemLink({
  to,
  label,
  icon: Icon,
  collapsed,
}: NavItem & { collapsed: boolean }) {
  return (
    <NavLink
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
  );
}

export function Sidebar() {
  const collapsed = useUIStore((s) => s.sidebarCollapsed);
  const logout = useAuthStore((s) => s.logout);
  const user = useAuthStore((s) => s.user);

  const isAdmin = user?.role === 'admin';

  return (
    <aside
      className="flex flex-col bg-bg-secondary border-r border-gray-700 transition-all duration-200"
      style={{ width: collapsed ? 64 : 240 }}
    >
      <nav className="flex-1 flex flex-col p-2 mt-2 gap-4">
        {navSections.map((section) => {
          if (section.adminOnly && !isAdmin) {
            return null;
          }

          return (
            <div key={section.title} className="flex flex-col gap-1">
              {!collapsed && (
                <span className="px-3 py-1 text-xs font-semibold uppercase tracking-wider text-text-secondary select-none">
                  {section.title}
                </span>
              )}
              {section.items.map((item) => (
                <NavItemLink key={item.to} {...item} collapsed={collapsed} />
              ))}
            </div>
          );
        })}
      </nav>

      <div className="p-2 border-t border-gray-700">
        <button
          onClick={() => logout()}
          className="flex items-center gap-3 px-3 py-2 rounded-md text-sm text-text-secondary hover:text-status-error hover:bg-bg-tertiary/50 w-full transition-colors"
          aria-label="Sign out"
        >
          <LogOut size={20} className="shrink-0" />
          {!collapsed && <span>Sign out</span>}
        </button>
      </div>
    </aside>
  );
}
