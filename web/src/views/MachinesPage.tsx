import { useState, useMemo } from 'react';
import { Server, AlertCircle, RefreshCw } from 'lucide-react';
import { useMachines } from '../hooks/useMachines.ts';
import { useEventStream } from '../hooks/useEventStream.ts';
import { MachineCard } from '../components/machines/MachineCard.tsx';
import { EmptyState } from '../components/shared/EmptyState.tsx';
import { NewSessionModal } from '../components/sessions/NewSessionModal.tsx';

export function MachinesPage() {
  useEventStream();
  const { data: machines, isLoading, error, refetch } = useMachines();

  const [modalOpen, setModalOpen] = useState(false);
  const [preselectedMachine, setPreselectedMachine] = useState<string | undefined>();

  const counts = useMemo(() => {
    const all = machines ?? [];
    return {
      online: all.filter((m) => m.status === 'online').length,
      offline: all.filter((m) => m.status === 'offline').length,
      total: all.length,
    };
  }, [machines]);

  function handleCreateSession(machineId: string) {
    setPreselectedMachine(machineId);
    setModalOpen(true);
  }

  if (error) {
    return (
      <div className="p-6">
        <div className="bg-status-error/10 border border-status-error/30 rounded-lg p-4 flex items-center gap-3">
          <AlertCircle className="text-status-error shrink-0" size={20} />
          <p className="text-sm text-text-primary flex-1">
            {error instanceof Error ? error.message : 'Failed to load machines'}
          </p>
          <button
            onClick={() => refetch()}
            className="flex items-center gap-1.5 px-3 py-1.5 text-xs rounded-md bg-bg-tertiary text-text-secondary hover:text-text-primary transition-colors"
          >
            <RefreshCw size={14} />
            Retry
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className="p-6 space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <h1 className="text-xl font-semibold text-text-primary">Machines</h1>
        {!isLoading && (
          <div className="flex items-center gap-3 text-sm">
            <span className="text-status-success">{counts.online} online</span>
            <span className="text-text-secondary">/</span>
            <span className="text-text-secondary">{counts.offline} offline</span>
          </div>
        )}
      </div>

      {/* Loading State */}
      {isLoading && (
        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
          {Array.from({ length: 3 }, (_, i) => (
            <div key={i} className="bg-bg-tertiary rounded-lg p-4 animate-pulse">
              <div className="h-4 bg-bg-secondary rounded w-1/3 mb-3" />
              <div className="h-3 bg-bg-secondary rounded w-2/3 mb-2" />
              <div className="h-3 bg-bg-secondary rounded w-1/2" />
            </div>
          ))}
        </div>
      )}

      {/* Empty State */}
      {!isLoading && (machines ?? []).length === 0 && (
        <EmptyState
          icon={<Server size={48} />}
          title="No machines registered"
          description="Machines will appear here once agents connect to the server."
        />
      )}

      {/* Machine Grid */}
      {!isLoading && (machines ?? []).length > 0 && (
        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
          {(machines ?? []).map((machine) => (
            <MachineCard
              key={machine.machine_id}
              machine={machine}
              onCreateSession={handleCreateSession}
            />
          ))}
        </div>
      )}

      <NewSessionModal
        open={modalOpen}
        onClose={() => setModalOpen(false)}
        preselectedMachineId={preselectedMachine}
      />
    </div>
  );
}
