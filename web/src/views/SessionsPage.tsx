import { useState, useMemo, useCallback, useEffect } from 'react';
import { useNavigate } from 'react-router';
import { Plus, RefreshCw, AlertCircle, LayoutGrid, Search } from 'lucide-react';
import { toast } from 'sonner';
import { useSessions, useTerminateSession } from '../hooks/useSessions.ts';
import { useMachines } from '../hooks/useMachines.ts';
import { SessionList } from '../components/sessions/SessionList.tsx';
import { NewSessionModal } from '../components/sessions/NewSessionModal.tsx';
import { ConfirmDialog } from '../components/shared/ConfirmDialog.tsx';
import { Pagination } from '../components/shared/Pagination.tsx';
import { usePagination } from '../hooks/usePagination.ts';
import { useMultiviewStore } from '../stores/multiview.ts';

const STATUS_OPTIONS = ['all', 'running', 'created', 'completed', 'failed', 'terminated'] as const;

export function SessionsPage() {
  const navigate = useNavigate();

  const [statusFilter, setStatusFilter] = useState<string>('running');
  const [machineFilter, setMachineFilter] = useState<string>('all');
  const [modalOpen, setModalOpen] = useState(false);
  const [terminateId, setTerminateId] = useState<string | null>(null);
  const [search, setSearch] = useState('');
  const [multiSelectMode, setMultiSelectMode] = useState(false);
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());
  const createScratchWorkspace = useMultiviewStore((s) => s.createScratchWorkspace);

  const filters = useMemo(() => {
    const f: Record<string, string> = {};
    if (statusFilter !== 'all') f.status = statusFilter;
    if (machineFilter !== 'all') f.machine_id = machineFilter;
    return Object.keys(f).length > 0 ? f : undefined;
  }, [statusFilter, machineFilter]);

  const { data: sessions, isLoading, error, refetch } = useSessions(filters);
  const { data: machines } = useMachines();
  const terminateSession = useTerminateSession();

  const filteredSessions = useMemo(() => {
    const list = sessions ?? [];
    if (!search.trim()) return list;
    const q = search.toLowerCase();
    return list.filter(
      (s) =>
        s.session_id.toLowerCase().includes(q) ||
        s.command?.toLowerCase().includes(q) ||
        s.working_dir?.toLowerCase().includes(q),
    );
  }, [sessions, search]);

  const { paged: pagedSessions, page, pageSize, total, setPage, setPageSize } = usePagination(filteredSessions);

  useEffect(() => { setPage(1); }, [search, statusFilter, machineFilter, setPage]);

  function handleAttach(id: string) {
    navigate(`/sessions/${id}`);
  }

  function handleTerminate(id: string) {
    setTerminateId(id);
  }

  const handleSelect = useCallback((id: string) => {
    setSelectedIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) {
        next.delete(id);
      } else if (next.size < 6) {
        next.add(id);
      }
      return next;
    });
  }, []);

  const handleOpenMultiView = useCallback(() => {
    if (selectedIds.size >= 2) {
      createScratchWorkspace([...selectedIds]);
      navigate('/multiview');
    }
  }, [selectedIds, createScratchWorkspace, navigate]);

  async function confirmTerminate() {
    if (!terminateId) return;
    try {
      await terminateSession.mutateAsync(terminateId);
      toast.success('Session terminated');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to terminate session');
    }
    setTerminateId(null);
  }

  return (
    <div className="p-4 md:p-6 space-y-6">
      {/* Header */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <h1 className="text-xl font-semibold text-text-primary">Sessions</h1>
        <div className="flex items-center gap-2">
          <button
            onClick={() => {
              setMultiSelectMode(!multiSelectMode);
              setSelectedIds(new Set());
            }}
            className={`flex items-center gap-1.5 px-3 py-2 text-sm rounded-md transition-colors ${
              multiSelectMode
                ? 'bg-accent-primary text-white'
                : 'bg-bg-secondary border border-border-primary text-text-primary hover:bg-bg-tertiary'
            }`}
          >
            <LayoutGrid size={14} />
            Multi-View
          </button>
          <button
            onClick={() => setModalOpen(true)}
            className="flex items-center gap-2 px-4 py-2 text-sm rounded-md bg-accent-primary hover:bg-accent-primary/80 text-white transition-colors"
          >
            <Plus size={16} />
            New Session
          </button>
        </div>
      </div>

      {/* Search & Filters */}
      <div className="flex flex-wrap items-center gap-4">
        <div className="relative">
          <Search size={14} className="absolute left-2.5 top-1/2 -translate-y-1/2 text-text-secondary pointer-events-none" />
          <input
            type="text"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search sessions..."
            aria-label="Search sessions by ID, command, or directory"
            className="rounded-md bg-bg-tertiary border border-gray-600 text-text-primary text-sm pl-8 pr-3 py-1.5 focus:outline-none focus:ring-1 focus:ring-accent-primary w-56"
          />
        </div>
        <div>
          <label className="block text-xs text-text-secondary mb-1">Status</label>
          <select
            value={statusFilter}
            onChange={(e) => setStatusFilter(e.target.value)}
            className="rounded-md bg-bg-tertiary border border-gray-600 text-text-primary text-sm px-3 py-1.5 focus:outline-none focus:ring-1 focus:ring-accent-primary"
          >
            {STATUS_OPTIONS.map((s) => (
              <option key={s} value={s}>
                {s === 'all' ? 'All Statuses' : s.charAt(0).toUpperCase() + s.slice(1)}
              </option>
            ))}
          </select>
        </div>
        <div>
          <label className="block text-xs text-text-secondary mb-1">Machine</label>
          <select
            value={machineFilter}
            onChange={(e) => setMachineFilter(e.target.value)}
            className="rounded-md bg-bg-tertiary border border-gray-600 text-text-primary text-sm px-3 py-1.5 focus:outline-none focus:ring-1 focus:ring-accent-primary"
          >
            <option value="all">All Machines</option>
            {(machines ?? []).map((m) => (
              <option key={m.machine_id} value={m.machine_id}>
                {m.display_name || m.machine_id.slice(0, 8)}
              </option>
            ))}
          </select>
        </div>
      </div>

      {/* Error State */}
      {error && (
        <div className="bg-status-error/10 border border-status-error/30 rounded-lg p-4 flex items-center gap-3">
          <AlertCircle className="text-status-error shrink-0" size={20} />
          <p className="text-sm text-text-primary flex-1">
            {error instanceof Error ? error.message : 'Failed to load sessions'}
          </p>
          <button
            onClick={() => refetch()}
            className="flex items-center gap-1.5 px-3 py-1.5 text-xs rounded-md bg-bg-tertiary text-text-secondary hover:text-text-primary transition-colors"
          >
            <RefreshCw size={14} />
            Retry
          </button>
        </div>
      )}

      {/* Loading State */}
      {isLoading && (
        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
          {Array.from({ length: 6 }, (_, i) => (
            <div key={i} className="bg-bg-tertiary rounded-lg p-4 animate-pulse">
              <div className="h-4 bg-bg-secondary rounded w-1/3 mb-3" />
              <div className="h-3 bg-bg-secondary rounded w-2/3 mb-2" />
              <div className="h-3 bg-bg-secondary rounded w-1/2" />
            </div>
          ))}
        </div>
      )}

      {/* Session List */}
      {!isLoading && !error && (
        <>
          <SessionList
            sessions={pagedSessions}
            onAttach={handleAttach}
            onTerminate={handleTerminate}
            selectable={multiSelectMode}
            selectedIds={selectedIds}
            onSelect={handleSelect}
          />
          <Pagination
            page={page}
            pageSize={pageSize}
            total={total}
            onPageChange={setPage}
            onPageSizeChange={setPageSize}
          />
        </>
      )}

      <NewSessionModal open={modalOpen} onClose={() => setModalOpen(false)} />

      {multiSelectMode && selectedIds.size >= 2 && (
        <div className="fixed bottom-6 left-1/2 -translate-x-1/2 bg-bg-secondary border border-border-primary rounded-lg shadow-xl px-4 py-2.5 flex items-center gap-3 z-40">
          <span className="text-sm text-text-secondary">{selectedIds.size} sessions selected</span>
          <button
            onClick={handleOpenMultiView}
            className="flex items-center gap-1.5 px-3 py-1.5 text-sm rounded bg-accent-primary text-white hover:bg-accent-primary/80 transition-colors"
          >
            <LayoutGrid size={14} />
            Open in Multi-View
          </button>
        </div>
      )}

      <ConfirmDialog
        open={terminateId !== null}
        title="Terminate Session"
        message="Are you sure you want to terminate this session? This action cannot be undone."
        confirmLabel="Terminate"
        variant="danger"
        onConfirm={confirmTerminate}
        onCancel={() => setTerminateId(null)}
      />
    </div>
  );
}
