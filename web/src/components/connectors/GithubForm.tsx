import { useState, useEffect, useCallback } from 'react';
import { createPortal } from 'react-dom';
import { X, Plus } from 'lucide-react';
import { toast } from 'sonner';
import { useCreateConnector, useUpdateConnector } from '../../hooks/useBridge.ts';
import type { BridgeConnector } from '../../types/connector.ts';
import { WatchEditor } from './WatchEditor.tsx';
import type { WatchData } from './WatchEditor.tsx';
import { createDefaultWatch } from './watchDefaults.ts';

interface GithubConfig {
  watches: WatchData[];
}

interface GithubFormProps {
  connector?: BridgeConnector;
  onClose: () => void;
  onSaved?: () => void;
}

function parseGithubConfig(configJson: string): Partial<GithubConfig> {
  try {
    return JSON.parse(configJson) as Partial<GithubConfig>;
  } catch {
    return {};
  }
}

function buildConfigJson(watches: WatchData[]): string {
  const serialized = watches.map((w) => ({
    repo: w.repo,
    template: w.template,
    machine_id: w.machine_id,
    poll_interval: w.poll_interval,
    triggers: {
      pull_request_opened: w.triggers.pull_request_opened.enabled
        ? { filters: w.triggers.pull_request_opened.filters }
        : null,
      check_run_completed: w.triggers.check_run_completed.enabled
        ? { filters: w.triggers.check_run_completed.filters }
        : null,
      issue_labeled: w.triggers.issue_labeled.enabled
        ? { filters: w.triggers.issue_labeled.filters }
        : null,
    },
  }));
  return JSON.stringify({ watches: serialized });
}

function hydrateWatches(raw: unknown): WatchData[] {
  if (!Array.isArray(raw)) return [];
  return raw.map((w) => {
    const item = w as Record<string, unknown>;
    const triggers = (item.triggers ?? {}) as Record<string, unknown>;

    function hydrateT(key: string) {
      const t = triggers[key];
      if (!t || typeof t !== 'object') return { enabled: false, filters: {} };
      return { enabled: true, filters: (t as Record<string, unknown>).filters ?? {} };
    }

    return {
      repo: String(item.repo ?? ''),
      template: String(item.template ?? ''),
      machine_id: String(item.machine_id ?? ''),
      poll_interval: String(item.poll_interval ?? '60s'),
      triggers: {
        pull_request_opened: hydrateT('pull_request_opened'),
        check_run_completed: hydrateT('check_run_completed'),
        issue_labeled: hydrateT('issue_labeled'),
      },
    };
  });
}

const inputClass =
  'w-full rounded-md bg-bg-tertiary border border-gray-600 text-text-primary text-sm px-3 py-2 focus:outline-none focus:ring-1 focus:ring-accent-primary placeholder:text-text-secondary/30';

export function GithubForm({ connector, onClose, onSaved }: GithubFormProps) {
  const isEdit = connector !== undefined;
  const existingConfig = isEdit ? parseGithubConfig(connector.config) : {};

  const [name, setName] = useState(connector?.name ?? '');
  const [token, setToken] = useState('');
  const [watches, setWatches] = useState<WatchData[]>(() => {
    const hydrated = hydrateWatches(existingConfig.watches);
    return hydrated.length > 0 ? hydrated : [createDefaultWatch()];
  });

  const createConnector = useCreateConnector();
  const updateConnector = useUpdateConnector();
  const isPending = createConnector.isPending || updateConnector.isPending;

  const handleKeyDown = useCallback(
    (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        onClose();
      }
    },
    [onClose],
  );

  useEffect(() => {
    document.addEventListener('keydown', handleKeyDown);
    return () => document.removeEventListener('keydown', handleKeyDown);
  }, [handleKeyDown]);

  function addWatch() {
    setWatches((prev) => [...prev, createDefaultWatch()]);
  }

  function updateWatch(index: number, updated: WatchData) {
    setWatches((prev) => prev.map((w, i) => (i === index ? updated : w)));
  }

  function removeWatch(index: number) {
    setWatches((prev) => prev.filter((_, i) => i !== index));
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();

    const configJson = buildConfigJson(watches);
    const configSecret =
      token.trim() ? JSON.stringify({ token: token.trim() }) : undefined;

    try {
      if (isEdit) {
        await updateConnector.mutateAsync({
          id: connector.connector_id,
          params: {
            connector_type: 'github',
            name: name.trim(),
            config: configJson,
            ...(configSecret !== undefined ? { config_secret: configSecret } : {}),
          },
        });
        toast.success('Connector updated');
      } else {
        await createConnector.mutateAsync({
          connector_type: 'github',
          name: name.trim(),
          config: configJson,
          ...(configSecret !== undefined ? { config_secret: configSecret } : {}),
        });
        toast.success('Connector created');
      }

      if (onSaved) {
        onSaved();
      } else {
        onClose();
      }
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to save connector');
    }
  }

  return createPortal(
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      <div className="absolute inset-0 bg-black/60" onClick={onClose} />
      <div className="relative bg-bg-secondary border border-border-primary rounded-lg shadow-xl max-w-2xl w-full mx-4 p-6 max-h-[90vh] overflow-y-auto">
        {/* Header */}
        <div className="flex items-center justify-between mb-5">
          <h2 className="text-lg font-semibold text-text-primary">
            {isEdit ? 'Edit GitHub Connector' : 'New GitHub Connector'}
          </h2>
          <button
            onClick={onClose}
            className="p-1.5 rounded-md text-text-secondary hover:text-text-primary hover:bg-bg-tertiary transition-colors"
          >
            <X size={16} />
          </button>
        </div>

        <form onSubmit={handleSubmit} className="flex flex-col gap-5">
          {/* Connector name */}
          <div>
            <label className="block text-sm text-text-secondary mb-1">
              Connector name <span className="text-status-error">*</span>
            </label>
            <input
              type="text"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="e.g. my-github"
              required
              autoFocus
              className={inputClass}
            />
          </div>

          {/* GitHub token */}
          <div>
            <label className="block text-sm text-text-secondary mb-1">
              GitHub token{' '}
              {!isEdit && <span className="text-status-error">*</span>}
              {isEdit && (
                <span className="text-text-secondary/50 ml-1">
                  (leave blank to keep existing)
                </span>
              )}
            </label>
            <input
              type="password"
              value={token}
              onChange={(e) => setToken(e.target.value)}
              placeholder="ghp_..."
              required={!isEdit}
              className={inputClass}
            />
            <p className="mt-1 text-xs text-text-secondary/50">
              Requires <code className="font-mono">repo</code> scope for PR, check run, and
              issue access.
            </p>
          </div>

          {/* Watches */}
          <div>
            <div className="flex items-center justify-between mb-2">
              <p className="text-sm text-text-secondary">Watches</p>
              <button
                type="button"
                onClick={addWatch}
                className="flex items-center gap-1 text-xs text-accent-primary hover:text-accent-primary/80 transition-colors"
              >
                <Plus size={13} />
                Add Watch
              </button>
            </div>

            {watches.length === 0 ? (
              <p className="text-xs text-text-secondary/50 italic">
                No watches configured. Add a watch to monitor a repository.
              </p>
            ) : (
              <div className="flex flex-col gap-3">
                {watches.map((watch, i) => (
                  <WatchEditor
                    key={i}
                    index={i}
                    watch={watch}
                    onChange={(updated) => updateWatch(i, updated)}
                    onRemove={() => removeWatch(i)}
                  />
                ))}
              </div>
            )}
          </div>

          {/* Actions */}
          <div className="flex justify-end gap-3 mt-1">
            <button
              type="button"
              onClick={onClose}
              className="px-4 py-2 text-sm rounded-md text-text-secondary hover:text-text-primary bg-bg-tertiary hover:bg-bg-tertiary/80 transition-colors"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={isPending}
              className="px-4 py-2 text-sm rounded-md bg-accent-primary hover:bg-accent-primary/80 text-white transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {isPending ? 'Saving...' : isEdit ? 'Save changes' : 'Create connector'}
            </button>
          </div>
        </form>
      </div>
    </div>,
    document.body,
  );
}
