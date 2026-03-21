import { Search, Radio } from 'lucide-react';
import { useLogsStore } from '../../stores/logs.ts';

const LEVELS = ['ALL', 'DEBUG', 'INFO', 'WARN', 'ERROR'] as const;
const SOURCES = ['ALL', 'server', 'agent', 'bridge'] as const;
const COMPONENTS = [
  'ALL',
  'grpc',
  'session',
  'connmgr',
  'auth',
  'orchestrator',
  'scheduler',
  'event',
] as const;

interface TimePreset {
  label: string;
  hours: number;
}

const TIME_PRESETS: TimePreset[] = [
  { label: '1h', hours: 1 },
  { label: '6h', hours: 6 },
  { label: '24h', hours: 24 },
  { label: '7d', hours: 168 },
];

function hoursAgoISO(hours: number): string {
  return new Date(Date.now() - hours * 3600_000).toISOString();
}

export function LogFilters() {
  const filter = useLogsStore((s) => s.filter);
  const live = useLogsStore((s) => s.live);
  const setFilter = useLogsStore((s) => s.setFilter);
  const setLive = useLogsStore((s) => s.setLive);

  const selectClass =
    'rounded-md bg-bg-tertiary border border-gray-600 text-text-primary text-sm px-3 py-1.5 focus:outline-none focus:ring-1 focus:ring-accent-primary';

  return (
    <div className="flex flex-wrap items-end gap-3">
      {/* Level */}
      <div>
        <label className="block text-xs text-text-secondary mb-1">Level</label>
        <select
          value={filter.level ?? 'ALL'}
          onChange={(e) =>
            setFilter({ level: e.target.value === 'ALL' ? undefined : e.target.value })
          }
          className={selectClass}
        >
          {LEVELS.map((l) => (
            <option key={l} value={l}>
              {l === 'ALL' ? 'All Levels' : l}
            </option>
          ))}
        </select>
      </div>

      {/* Source */}
      <div>
        <label className="block text-xs text-text-secondary mb-1">Source</label>
        <select
          value={filter.source ?? 'ALL'}
          onChange={(e) =>
            setFilter({ source: e.target.value === 'ALL' ? undefined : e.target.value })
          }
          className={selectClass}
        >
          {SOURCES.map((s) => (
            <option key={s} value={s}>
              {s === 'ALL' ? 'All Sources' : s.charAt(0).toUpperCase() + s.slice(1)}
            </option>
          ))}
        </select>
      </div>

      {/* Component */}
      <div>
        <label className="block text-xs text-text-secondary mb-1">Component</label>
        <select
          value={filter.component ?? 'ALL'}
          onChange={(e) =>
            setFilter({ component: e.target.value === 'ALL' ? undefined : e.target.value })
          }
          className={selectClass}
        >
          {COMPONENTS.map((c) => (
            <option key={c} value={c}>
              {c === 'ALL' ? 'All Components' : c}
            </option>
          ))}
        </select>
      </div>

      {/* Machine ID */}
      <div>
        <label className="block text-xs text-text-secondary mb-1">Machine</label>
        <input
          type="text"
          placeholder="Machine ID"
          value={filter.machine_id ?? ''}
          onChange={(e) =>
            setFilter({ machine_id: e.target.value || undefined })
          }
          className="rounded-md bg-bg-tertiary border border-gray-600 text-text-primary text-sm px-3 py-1.5 focus:outline-none focus:ring-1 focus:ring-accent-primary w-36"
        />
      </div>

      {/* Search */}
      <div className="relative flex-1 min-w-48">
        <label className="block text-xs text-text-secondary mb-1">Search</label>
        <div className="relative">
          <Search
            size={14}
            className="absolute left-2.5 top-1/2 -translate-y-1/2 text-text-secondary pointer-events-none"
          />
          <input
            type="text"
            placeholder="Search log messages..."
            value={filter.search ?? ''}
            onChange={(e) => setFilter({ search: e.target.value || undefined })}
            className="w-full rounded-md bg-bg-tertiary border border-gray-600 text-text-primary text-sm pl-8 pr-3 py-1.5 focus:outline-none focus:ring-1 focus:ring-accent-primary"
          />
        </div>
      </div>

      {/* Time presets */}
      <div>
        <label className="block text-xs text-text-secondary mb-1">Time Range</label>
        <div className="flex items-center gap-1">
          {TIME_PRESETS.map((preset) => (
            <button
              key={preset.label}
              type="button"
              onClick={() => setFilter({ since: hoursAgoISO(preset.hours), until: undefined })}
              className="px-2.5 py-1.5 text-xs rounded-md bg-bg-secondary border border-border-primary text-text-secondary hover:text-text-primary hover:border-gray-500 transition-colors"
            >
              {preset.label}
            </button>
          ))}
        </div>
      </div>

      {/* Live toggle */}
      <button
        type="button"
        onClick={() => setLive(!live)}
        className={`flex items-center gap-1.5 px-3 py-1.5 text-sm rounded-md transition-colors ${
          live
            ? 'bg-green-600/20 border border-green-500/50 text-green-400'
            : 'bg-bg-secondary border border-border-primary text-text-secondary hover:text-text-primary'
        }`}
      >
        <Radio size={14} className={live ? 'animate-pulse' : ''} />
        Live
      </button>
    </div>
  );
}
