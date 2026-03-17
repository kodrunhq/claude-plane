import { useEffect, useState, useCallback } from 'react';
import { Command } from 'cmdk';
import { useNavigate } from 'react-router';
import { useQueryClient } from '@tanstack/react-query';
import {
  LayoutDashboard,
  Terminal,
  Server,
  Workflow,
  Play,
  FileText,
  Webhook,
  Activity,
  Users,
  KeyRound,
  Lock,
  Plug,
  Zap,
  Clock,
  Key,
  Search,
  Settings,
  LayoutGrid,
  BookOpen,
  Plus,
} from 'lucide-react';
import type { Job } from '../../types/job.ts';
import type { Session } from '../../types/session.ts';
import type { Machine } from '../../lib/types.ts';
import type { SessionTemplate } from '../../types/template.ts';

interface SearchResult {
  id: string;
  label: string;
  description?: string;
  path: string;
  group: string;
}

const NAV_ITEMS = [
  { label: 'Command Center', path: '/', icon: LayoutDashboard },
  { label: 'Sessions', path: '/sessions', icon: Terminal },
  { label: 'Multi-View', path: '/multiview', icon: LayoutGrid },
  { label: 'Machines', path: '/machines', icon: Server },
  { label: 'Jobs', path: '/jobs', icon: Workflow },
  { label: 'Templates', path: '/templates', icon: FileText },
  { label: 'Runs', path: '/runs', icon: Play },
  { label: 'Search', path: '/search', icon: Search },
  { label: 'Webhooks', path: '/webhooks', icon: Webhook },
  { label: 'Triggers', path: '/triggers', icon: Zap },
  { label: 'Schedules', path: '/schedules', icon: Clock },
  { label: 'Connectors', path: '/connectors', icon: Plug },
  { label: 'Events', path: '/events', icon: Activity },
  { label: 'Credentials', path: '/credentials', icon: Lock },
  { label: 'Users', path: '/users', icon: Users },
  { label: 'Provisioning', path: '/provisioning', icon: KeyRound },
  { label: 'API Keys', path: '/api-keys', icon: Key },
  { label: 'Settings', path: '/settings', icon: Settings },
  { label: 'Documentation', path: '/docs', icon: BookOpen },
];

const QUICK_ACTIONS = [
  { label: 'New Session', path: '/sessions', icon: Plus },
  { label: 'New Job', path: '/jobs/new', icon: Plus },
  { label: 'New Template', path: '/templates/new', icon: Plus },
];

export function CommandPalette() {
  const [open, setOpen] = useState(false);
  const navigate = useNavigate();
  const queryClient = useQueryClient();

  const handleKeyDown = useCallback((e: KeyboardEvent) => {
    if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
      e.preventDefault();
      setOpen((prev) => !prev);
    }
  }, []);

  useEffect(() => {
    document.addEventListener('keydown', handleKeyDown);
    return () => document.removeEventListener('keydown', handleKeyDown);
  }, [handleKeyDown]);

  const cachedResults = (): SearchResult[] => {
    const results: SearchResult[] = [];

    const jobs = queryClient.getQueryData<Job[]>(['jobs']);
    if (jobs) {
      for (const job of jobs) {
        results.push({
          id: job.job_id,
          label: job.name,
          description: job.description || undefined,
          path: `/jobs/${job.job_id}`,
          group: 'Jobs',
        });
      }
    }

    const sessionsEntries = queryClient.getQueriesData<Session[]>({ queryKey: ['sessions'] });
    const sessions = sessionsEntries.flatMap(([, data]) => data ?? []);
    if (sessions) {
      for (const session of sessions) {
        results.push({
          id: session.session_id,
          label: `Session ${session.session_id.slice(0, 8)}`,
          description: `${session.status} on ${session.machine_id.slice(0, 12)}`,
          path: `/sessions/${session.session_id}`,
          group: 'Sessions',
        });
      }
    }

    const machines = queryClient.getQueryData<Machine[]>(['machines']);
    if (machines) {
      for (const machine of machines) {
        results.push({
          id: machine.machine_id,
          label: machine.display_name || machine.machine_id,
          description: machine.status,
          path: `/machines`,
          group: 'Machines',
        });
      }
    }

    const templateEntries = queryClient.getQueriesData<SessionTemplate[]>({ queryKey: ['templates'] });
    const templates = templateEntries.flatMap(([, data]) => data ?? []);
    if (templates) {
      for (const template of templates) {
        results.push({
          id: template.template_id,
          label: template.name,
          description: template.description || undefined,
          path: `/templates/${template.template_id}/edit`,
          group: 'Templates',
        });
      }
    }

    return results;
  };

  function handleSelect(path: string) {
    setOpen(false);
    navigate(path);
  }

  if (!open) return null;

  const results = cachedResults();
  const groups = new Map<string, SearchResult[]>();
  for (const r of results) {
    const existing = groups.get(r.group);
    if (existing) {
      existing.push(r);
    } else {
      groups.set(r.group, [r]);
    }
  }

  return (
    <div className="fixed inset-0 z-[100]">
      <div
        className="absolute inset-0 bg-black/60 backdrop-blur-sm"
        onClick={() => setOpen(false)}
        aria-hidden="true"
      />
      <div className="absolute top-[20%] left-1/2 -translate-x-1/2 w-full max-w-lg">
        <Command
          className="bg-bg-secondary border border-border-primary rounded-xl shadow-2xl overflow-hidden"
          label="Command palette"
        >
          <Command.Input
            placeholder="Type a command or search..."
            className="w-full px-4 py-3 text-sm text-text-primary bg-transparent border-b border-border-primary outline-none placeholder:text-text-secondary/50"
            autoFocus
          />
          <Command.List className="max-h-80 overflow-y-auto p-2">
            <Command.Empty className="px-4 py-6 text-sm text-text-secondary text-center">
              No results found.
            </Command.Empty>

            <Command.Group heading="Quick Actions" className="mb-2">
              <span className="px-3 py-1.5 text-[11px] font-semibold uppercase tracking-widest text-text-secondary/60">
                Quick Actions
              </span>
              {QUICK_ACTIONS.map((action) => (
                <Command.Item
                  key={action.path}
                  value={action.label}
                  onSelect={() => handleSelect(action.path)}
                  className="flex items-center gap-3 px-3 py-2 text-sm text-text-primary rounded-lg cursor-pointer data-[selected=true]:bg-accent-primary/10 data-[selected=true]:text-accent-primary transition-colors"
                >
                  <action.icon size={16} className="shrink-0 text-text-secondary" />
                  {action.label}
                </Command.Item>
              ))}
            </Command.Group>

            {Array.from(groups.entries()).map(([group, items]) => (
              <Command.Group key={group} heading={group} className="mb-2">
                <span className="px-3 py-1.5 text-[11px] font-semibold uppercase tracking-widest text-text-secondary/60">
                  {group}
                </span>
                {items.map((item) => (
                  <Command.Item
                    key={item.id}
                    value={`${item.label} ${item.description ?? ''}`}
                    onSelect={() => handleSelect(item.path)}
                    className="flex items-center gap-3 px-3 py-2 text-sm text-text-primary rounded-lg cursor-pointer data-[selected=true]:bg-accent-primary/10 data-[selected=true]:text-accent-primary transition-colors"
                  >
                    <div className="flex-1 min-w-0">
                      <div className="truncate">{item.label}</div>
                      {item.description && (
                        <div className="text-xs text-text-secondary truncate">{item.description}</div>
                      )}
                    </div>
                  </Command.Item>
                ))}
              </Command.Group>
            ))}

            <Command.Group heading="Navigation" className="mb-2">
              <span className="px-3 py-1.5 text-[11px] font-semibold uppercase tracking-widest text-text-secondary/60">
                Navigation
              </span>
              {NAV_ITEMS.map((item) => (
                <Command.Item
                  key={item.path}
                  value={`Go to ${item.label}`}
                  onSelect={() => handleSelect(item.path)}
                  className="flex items-center gap-3 px-3 py-2 text-sm text-text-primary rounded-lg cursor-pointer data-[selected=true]:bg-accent-primary/10 data-[selected=true]:text-accent-primary transition-colors"
                >
                  <item.icon size={16} className="shrink-0 text-text-secondary" />
                  {item.label}
                </Command.Item>
              ))}
            </Command.Group>
          </Command.List>
        </Command>
      </div>
    </div>
  );
}
