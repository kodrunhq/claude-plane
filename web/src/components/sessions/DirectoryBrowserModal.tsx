import { useState, useEffect, useCallback } from 'react';
import { createPortal } from 'react-dom';
import { Folder, File, ChevronRight, Loader2 } from 'lucide-react';
import { browseMachineDirectory } from '../../api/machines.ts';
import type { BrowseEntry } from '../../api/machines.ts';

interface DirectoryBrowserModalProps {
  open: boolean;
  onClose: () => void;
  onSelect: (path: string) => void;
  machineId: string;
  initialPath?: string;
}

export function DirectoryBrowserModal({
  open,
  onClose,
  onSelect,
  machineId,
  initialPath,
}: DirectoryBrowserModalProps) {
  const [currentPath, setCurrentPath] = useState(initialPath ?? '/');
  const [parentPath, setParentPath] = useState<string | null>(null);
  const [entries, setEntries] = useState<BrowseEntry[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const fetchEntries = useCallback(async (path: string) => {
    setLoading(true);
    setError(null);
    try {
      const response = await browseMachineDirectory(machineId, path);
      setCurrentPath(response.path);
      setParentPath(response.parent || null);
      setEntries(response.entries);
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to browse directory';
      setError(message);
      setEntries([]);
    } finally {
      setLoading(false);
    }
  }, [machineId]);

  useEffect(() => {
    if (open) {
      const path = initialPath ?? '/';
      setCurrentPath(path);
      void fetchEntries(path);
    }
  }, [open, initialPath, fetchEntries]);

  useEffect(() => {
    if (!open) return;
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    document.addEventListener('keydown', handleKeyDown);
    return () => document.removeEventListener('keydown', handleKeyDown);
  }, [open, onClose]);

  function navigateTo(path: string) {
    setCurrentPath(path);
    void fetchEntries(path);
  }

  // Build breadcrumb segments from the current path
  const pathSegments = currentPath.split('/').filter(Boolean);
  const breadcrumbs = pathSegments.map((segment, index) => ({
    label: segment,
    path: '/' + pathSegments.slice(0, index + 1).join('/'),
  }));

  const dirs = entries.filter((e) => e.type === 'dir');
  const files = entries.filter((e) => e.type === 'file');

  if (!open) return null;

  return createPortal(
    <div className="fixed inset-0 z-[60] flex items-center justify-center">
      <div className="absolute inset-0 bg-black/60" onClick={onClose} />
      <div className="relative bg-bg-secondary border border-border-primary rounded-lg shadow-xl max-w-lg w-full mx-4 flex flex-col" style={{ maxHeight: '70vh' }}>
        {/* Header */}
        <div className="p-4 border-b border-border-primary">
          <h3 className="text-sm font-semibold text-text-primary mb-2">Browse Directory</h3>

          {/* Breadcrumb */}
          <div className="flex items-center gap-1 text-xs text-text-secondary overflow-x-auto">
            <button
              type="button"
              onClick={() => navigateTo('/')}
              className="hover:text-text-primary transition-colors shrink-0"
            >
              /
            </button>
            {breadcrumbs.map((crumb) => (
              <span key={crumb.path} className="flex items-center gap-1 shrink-0">
                <ChevronRight size={12} className="text-text-secondary/50" />
                <button
                  type="button"
                  onClick={() => navigateTo(crumb.path)}
                  className="hover:text-text-primary transition-colors"
                >
                  {crumb.label}
                </button>
              </span>
            ))}
          </div>
        </div>

        {/* Content */}
        <div className="flex-1 overflow-y-auto p-2">
          {loading && (
            <div className="flex items-center justify-center py-8">
              <Loader2 size={20} className="animate-spin text-text-secondary" />
            </div>
          )}

          {error && !loading && (
            <div className="text-sm text-status-error px-2 py-4">{error}</div>
          )}

          {!loading && !error && (
            <div className="flex flex-col">
              {parentPath !== null && parentPath !== '' && (
                <button
                  type="button"
                  onClick={() => navigateTo(parentPath)}
                  className="flex items-center gap-2 px-2 py-1.5 text-sm text-text-secondary hover:text-text-primary hover:bg-bg-tertiary rounded transition-colors"
                >
                  <Folder size={14} />
                  ..
                </button>
              )}
              {dirs.map((entry) => (
                <button
                  key={entry.name}
                  type="button"
                  onClick={() => navigateTo(currentPath === '/' ? `/${entry.name}` : `${currentPath}/${entry.name}`)}
                  className="flex items-center gap-2 px-2 py-1.5 text-sm text-text-primary hover:bg-bg-tertiary rounded transition-colors text-left"
                >
                  <Folder size={14} className="text-accent-primary shrink-0" />
                  {entry.name}
                </button>
              ))}
              {files.map((entry) => (
                <div
                  key={entry.name}
                  className="flex items-center gap-2 px-2 py-1.5 text-sm text-text-secondary/50"
                >
                  <File size={14} className="shrink-0" />
                  {entry.name}
                </div>
              ))}
              {dirs.length === 0 && files.length === 0 && (
                <p className="text-sm text-text-secondary/50 px-2 py-4">Empty directory</p>
              )}
            </div>
          )}
        </div>

        {/* Footer */}
        <div className="p-4 border-t border-border-primary flex items-center justify-between">
          <span className="text-xs text-text-secondary truncate mr-4">{currentPath}</span>
          <div className="flex gap-2 shrink-0">
            <button
              type="button"
              onClick={onClose}
              className="px-3 py-1.5 text-sm rounded-md text-text-secondary hover:text-text-primary bg-bg-tertiary hover:bg-bg-tertiary/80 transition-colors"
            >
              Cancel
            </button>
            <button
              type="button"
              onClick={() => onSelect(currentPath)}
              className="px-3 py-1.5 text-sm rounded-md bg-accent-primary hover:bg-accent-primary/80 text-white transition-colors"
            >
              Select
            </button>
          </div>
        </div>
      </div>
    </div>,
    document.body,
  );
}
