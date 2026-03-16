import { useState, useMemo, useEffect } from 'react';
import { X, Search, Sparkles, TerminalSquare } from 'lucide-react';
import { useSessions } from '../../hooks/useSessions';
import { useMachines } from '../../hooks/useMachines';
import { StatusBadge } from '../shared/StatusBadge';

interface SessionPickerProps {
  readonly onSelect: (sessionId: string) => void;
  readonly onClose: () => void;
  readonly excludeSessionIds?: readonly string[];
}

export function SessionPicker({ onSelect, onClose, excludeSessionIds = [] }: SessionPickerProps) {
  const [search, setSearch] = useState('');
  const [machineFilter, setMachineFilter] = useState('all');

  const { data: sessions } = useSessions({ status: 'running' });
  const { data: machines } = useMachines();

  const machineMap = useMemo(() => {
    const map = new Map<string, string>();
    machines?.forEach((m) => map.set(m.machine_id, m.display_name ?? m.machine_id.slice(0, 8)));
    return map;
  }, [machines]);

  const filtered = useMemo(() => {
    if (!sessions) return [];
    return sessions.filter((s) => {
      if (machineFilter !== 'all' && s.machine_id !== machineFilter) return false;
      if (search) {
        const term = search.toLowerCase();
        const name = machineMap.get(s.machine_id) ?? '';
        return (
          s.session_id.toLowerCase().includes(term) ||
          s.command.toLowerCase().includes(term) ||
          s.working_dir.toLowerCase().includes(term) ||
          name.toLowerCase().includes(term)
        );
      }
      return true;
    });
  }, [sessions, machineFilter, search, machineMap]);

  const isExcluded = (id: string) => excludeSessionIds.includes(id);

  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.stopPropagation();
        onClose();
      }
    };
    document.addEventListener('keydown', handler);
    return () => document.removeEventListener('keydown', handler);
  }, [onClose]);

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50" onClick={onClose}>
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="session-picker-title"
        className="bg-bg-secondary rounded-lg shadow-xl w-full max-w-lg max-h-[70vh] flex flex-col"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center justify-between p-4 border-b border-border-primary">
          <h3 id="session-picker-title" className="text-sm font-semibold text-text-primary">Select Session</h3>
          <button onClick={onClose} className="p-1 rounded hover:bg-bg-tertiary">
            <X size={16} className="text-text-secondary" />
          </button>
        </div>

        <div className="p-3 border-b border-border-primary space-y-2">
          <div className="relative">
            <Search size={14} className="absolute left-2.5 top-2.5 text-text-secondary" />
            <input
              type="text"
              placeholder="Search sessions..."
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className="w-full pl-8 pr-3 py-2 text-sm bg-bg-primary border border-border-primary rounded text-text-primary placeholder:text-text-secondary focus:outline-none focus:ring-1 focus:ring-accent-primary"
              autoFocus
            />
          </div>
          <select
            value={machineFilter}
            onChange={(e) => setMachineFilter(e.target.value)}
            className="w-full px-3 py-1.5 text-sm bg-bg-primary border border-border-primary rounded text-text-primary"
          >
            <option value="all">All machines</option>
            {machines?.map((m) => (
              <option key={m.machine_id} value={m.machine_id}>
                {m.display_name ?? m.machine_id.slice(0, 8)}
              </option>
            ))}
          </select>
        </div>

        <div className="flex-1 overflow-y-auto p-2">
          {filtered.length === 0 && (
            <p className="text-center text-text-secondary text-sm py-8">No running sessions found</p>
          )}
          {filtered.map((session) => {
            const excluded = isExcluded(session.session_id);
            const isClaudeSession = session.command.toLowerCase().includes('claude');
            return (
              <button
                key={session.session_id}
                onClick={() => !excluded && onSelect(session.session_id)}
                disabled={excluded}
                className={`w-full text-left px-3 py-2 rounded mb-1 flex items-center gap-2 text-sm transition-colors ${
                  excluded
                    ? 'opacity-40 cursor-not-allowed'
                    : 'hover:bg-bg-tertiary cursor-pointer'
                }`}
              >
                {isClaudeSession ? (
                  <Sparkles size={14} className="text-accent-primary shrink-0" />
                ) : (
                  <TerminalSquare size={14} className="text-text-secondary shrink-0" />
                )}
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2">
                    <span className="font-mono text-xs text-text-secondary">
                      {session.session_id.slice(0, 8)}
                    </span>
                    <StatusBadge status={session.status} />
                    {excluded && (
                      <span className="text-xs text-text-secondary">Already in view</span>
                    )}
                  </div>
                  <div className="text-xs text-text-secondary truncate mt-0.5">
                    {machineMap.get(session.machine_id) ?? session.machine_id.slice(0, 8)} &middot; {session.working_dir}
                  </div>
                </div>
              </button>
            );
          })}
        </div>
      </div>
    </div>
  );
}
