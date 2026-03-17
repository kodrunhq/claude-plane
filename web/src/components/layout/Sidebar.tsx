import { NavLink } from 'react-router';
import {
  LayoutDashboard,
  Terminal,
  Server,
  Workflow,
  Play,
  FileText,
  LogOut,
  Webhook,
  Activity,
  Users,
  KeyRound,
  Lock,
  Plug,
  Key,
  Search,
  Settings,
  LayoutGrid,
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
      { to: '/multiview', label: 'Multi-View', icon: LayoutGrid },
      { to: '/machines', label: 'Machines', icon: Server },
      { to: '/jobs', label: 'Jobs', icon: Workflow },
      { to: '/templates', label: 'Templates', icon: FileText },
      { to: '/runs', label: 'Runs', icon: Play },
      { to: '/search', label: 'Search', icon: Search },
    ],
  },
  {
    title: 'Automation',
    items: [
      { to: '/webhooks', label: 'Webhooks', icon: Webhook },
      { to: '/connectors', label: 'Connectors', icon: Plug },
    ],
  },
  {
    title: 'Monitoring',
    items: [
      { to: '/events', label: 'Events', icon: Activity },
      { to: '/credentials', label: 'Credentials', icon: Lock },
    ],
  },
  {
    title: 'Admin',
    adminOnly: true,
    items: [
      { to: '/users', label: 'Users', icon: Users },
      { to: '/provisioning', label: 'Provisioning', icon: KeyRound },
      { to: '/api-keys', label: 'API Keys', icon: Key },
    ],
  },
];

interface NavItemLinkProps extends NavItem {
  collapsed: boolean;
  onClick?: () => void;
}

function NavItemLink({
  to,
  label,
  icon: Icon,
  collapsed,
  onClick,
}: NavItemLinkProps) {
  return (
    <NavLink
      to={to}
      end={to === '/'}
      onClick={onClick}
      title={collapsed ? label : undefined}
      className={({ isActive }) =>
        `group relative flex items-center gap-3 px-3 py-2 rounded-lg text-sm transition-all duration-200 ${collapsed ? 'justify-center' : ''} ${
          isActive
            ? 'bg-accent-primary/10 text-accent-primary'
            : 'text-text-secondary hover:text-text-primary hover:bg-bg-tertiary/60'
        }`
      }
    >
      {({ isActive }) => (
        <>
          {isActive && (
            <span className="absolute left-0 top-1/2 -translate-y-1/2 w-[3px] h-4 rounded-r-full bg-accent-primary" />
          )}
          <Icon size={18} className="shrink-0" />
          {!collapsed && <span className="font-medium">{label}</span>}
        </>
      )}
    </NavLink>
  );
}

interface SidebarProps {
  /** Called after a nav link is clicked — used by mobile drawer to auto-close. */
  onNavigate?: () => void;
}

export function Sidebar({ onNavigate }: SidebarProps) {
  const collapsed = useUIStore((s) => s.sidebarCollapsed);
  const logout = useAuthStore((s) => s.logout);
  const user = useAuthStore((s) => s.user);

  const isAdmin = user?.role === 'admin';

  return (
    <aside
      className="flex flex-col bg-bg-secondary border-r border-border-primary transition-all duration-200 h-full"
      style={{ width: collapsed && !onNavigate ? 64 : 240 }}
    >
      <nav className="flex-1 flex flex-col p-2 mt-2 gap-5 overflow-y-auto">
        {navSections.map((section) => {
          if (section.adminOnly && !isAdmin) {
            return null;
          }

          return (
            <div key={section.title} className="flex flex-col gap-0.5">
              {(!collapsed || onNavigate) && (
                <span className="px-3 py-1 text-[11px] font-semibold uppercase tracking-widest text-text-secondary/60 select-none">
                  {section.title}
                </span>
              )}
              {section.items.map((item) => (
                <NavItemLink key={item.to} {...item} collapsed={collapsed && !onNavigate} onClick={onNavigate} />
              ))}
            </div>
          );
        })}
      </nav>

      <div className="p-2 border-t border-border-primary space-y-0.5">
        <NavItemLink to="/settings" label="Settings" icon={Settings} collapsed={collapsed && !onNavigate} onClick={onNavigate} />
        <button
          onClick={() => logout()}
          className="flex items-center gap-3 px-3 py-2 rounded-lg text-sm text-text-secondary hover:text-status-error hover:bg-status-error/10 w-full transition-all duration-200"
          aria-label="Sign out"
          title={collapsed && !onNavigate ? 'Sign out' : undefined}
        >
          <LogOut size={18} className="shrink-0" />
          {(!collapsed || onNavigate) && <span className="font-medium">Sign out</span>}
        </button>
      </div>
    </aside>
  );
}
