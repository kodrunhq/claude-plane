import { useState, useMemo } from 'react';
import { Link, useNavigate } from 'react-router';
import { Terminal, Server, Activity, Plus, AlertCircle, RefreshCw } from 'lucide-react';
import { useSessions, useTerminateSession } from '../hooks/useSessions.ts';
import { useMachines } from '../hooks/useMachines.ts';
import { useEventStream } from '../hooks/useEventStream.ts';
import { SessionList } from '../components/sessions/SessionList.tsx';
import { MachineCard } from '../components/machines/MachineCard.tsx';
import { NewSessionModal } from '../components/sessions/NewSessionModal.tsx';
import { ConfirmDialog } from '../components/shared/ConfirmDialog.tsx';
import { toast } from 'sonner';

export function CommandCenter() {
  const navigate = useNavigate();
  const { data: sessions, isLoading: sessionsLoading, error: sessionsError, refetch: refetchSessions } = useSessions();
  const { data: machines, isLoading: machinesLoading, error: machinesError, refetch: refetchMachines } = useMachines();
  const terminateSession = useTerminateSession();
  useEventStream();

  const [modalOpen, setModalOpen] = useState(false);
  const [preselectedMachine, setPreselectedMachine] = useState<string | undefined>();
  const [terminateId, setTerminateId] = useState<string | null>(null);

  const activeSessions = useMemo(
    () => (sessions ?? []).filter((s) => s.status === 'running' || s.status === 'created'),
    [sessions],
  );
  const onlineMachines = useMemo(
    () => (machines ?? []).filter((m) => m.status === 'online'),
    [machines],
  );

  const error = sessionsError || machinesError;
  const isLoading = sessionsLoading || machinesLoading;

  function handleAttach(id: string) {
    navigate(`/sessions/${id}`);
  }

  function handleTerminate(id: string) {
    setTerminateId(id);
  }

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
            {error instanceof Error ? error.message : 'Failed to load data'}
          </p>
          <button
            onClick={() => { refetchSessions(); refetchMachines(); }}
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
        <h1 className="text-xl font-semibold text-text-primary">Command Center</h1>
        <button
          onClick={() => { setPreselectedMachine(undefined); setModalOpen(true); }}
          className="flex items-center gap-2 px-4 py-2 text-sm rounded-md bg-accent-primary hover:bg-accent-primary/80 text-white transition-colors"
        >
          <Plus size={16} />
          New Session
        </button>
      </div>

      {/* Stats Row */}
      <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
        <StatCard
          icon={<Activity size={24} />}
          label="Active Sessions"
          value={isLoading ? '--' : String(activeSessions.length)}
        />
        <StatCard
          icon={<Server size={24} />}
          label="Online Machines"
          value={isLoading ? '--' : String(onlineMachines.length)}
        />
        <StatCard
          icon={<Terminal size={24} />}
          label="Total Sessions"
          value={isLoading ? '--' : String(sessions?.length ?? 0)}
        />
      </div>

      {/* Active Sessions */}
      <section>
        <div className="flex items-center justify-between mb-3">
          <h2 className="text-sm font-medium text-text-secondary uppercase tracking-wider">
            Active Sessions
          </h2>
          <Link
            to="/sessions"
            className="text-xs text-accent-primary hover:text-accent-primary/80 transition-colors"
          >
            View All Sessions
          </Link>
        </div>
        {isLoading ? (
          <SkeletonGrid count={3} />
        ) : (
          <SessionList
            sessions={activeSessions.slice(0, 6)}
            onAttach={handleAttach}
            onTerminate={handleTerminate}
            emptyMessage="No active sessions. Create one to get started."
          />
        )}
      </section>

      {/* Machines */}
      <section>
        <div className="flex items-center justify-between mb-3">
          <h2 className="text-sm font-medium text-text-secondary uppercase tracking-wider">
            Machines
          </h2>
          <Link
            to="/machines"
            className="text-xs text-accent-primary hover:text-accent-primary/80 transition-colors"
          >
            View All Machines
          </Link>
        </div>
        {isLoading ? (
          <SkeletonGrid count={3} />
        ) : (machines ?? []).length === 0 ? (
          <p className="text-sm text-text-secondary py-8 text-center">
            No machines registered yet.
          </p>
        ) : (
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
      </section>

      <NewSessionModal
        open={modalOpen}
        onClose={() => setModalOpen(false)}
        preselectedMachineId={preselectedMachine}
      />

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

function StatCard({ icon, label, value }: { icon: React.ReactNode; label: string; value: string }) {
  return (
    <div className="bg-bg-secondary rounded-lg p-4 flex items-center gap-4">
      <div className="text-accent-primary">{icon}</div>
      <div>
        <p className="text-2xl font-bold text-text-primary">{value}</p>
        <p className="text-xs text-text-secondary">{label}</p>
      </div>
    </div>
  );
}

function SkeletonGrid({ count }: { count: number }) {
  return (
    <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
      {Array.from({ length: count }, (_, i) => (
        <div key={i} className="bg-bg-tertiary rounded-lg p-4 animate-pulse">
          <div className="h-4 bg-bg-secondary rounded w-1/3 mb-3" />
          <div className="h-3 bg-bg-secondary rounded w-2/3 mb-2" />
          <div className="h-3 bg-bg-secondary rounded w-1/2" />
        </div>
      ))}
    </div>
  );
}
