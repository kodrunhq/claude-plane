import { useState, useMemo } from 'react';
import { Link, useNavigate } from 'react-router';
import { Terminal, Server, Activity, Plus, AlertCircle, RefreshCw, Workflow, Play, Clock } from 'lucide-react';
import { useSessions, useTerminateSession } from '../hooks/useSessions.ts';
import { useMachines } from '../hooks/useMachines.ts';
import { useJobs } from '../hooks/useJobs.ts';
import { useRuns } from '../hooks/useRuns.ts';
import { SessionList } from '../components/sessions/SessionList.tsx';
import { MachineCard } from '../components/machines/MachineCard.tsx';
import { NewSessionModal } from '../components/sessions/NewSessionModal.tsx';
import { ConfirmDialog } from '../components/shared/ConfirmDialog.tsx';
import { StatusBadge } from '../components/shared/StatusBadge.tsx';
import { TimeAgo } from '../components/shared/TimeAgo.tsx';
import { toast } from 'sonner';
import type { Job, Run } from '../types/job.ts';

export function CommandCenter() {
  const navigate = useNavigate();
  const { data: sessions, isLoading: sessionsLoading, error: sessionsError, refetch: refetchSessions } = useSessions();
  const { data: machines, isLoading: machinesLoading, error: machinesError, refetch: refetchMachines } = useMachines();
  const { data: jobs, isLoading: jobsLoading } = useJobs();
  const { data: runs, isLoading: runsLoading } = useRuns({ limit: 20 });
  const terminateSession = useTerminateSession();

  const [modalOpen, setModalOpen] = useState(false);
  const [preselectedMachine, setPreselectedMachine] = useState<string | undefined>();
  const [terminateId, setTerminateId] = useState<string | null>(null);

  const activeSessions = useMemo(
    () => (sessions ?? []).filter((s) => s.status === 'running' || s.status === 'created'),
    [sessions],
  );
  const onlineMachines = useMemo(
    () => (machines ?? []).filter((m) => m.status === 'connected'),
    [machines],
  );

  const recentJobs = useMemo(
    () => [...(jobs ?? [])].sort((a, b) => b.updated_at.localeCompare(a.updated_at)).slice(0, 5),
    [jobs],
  );

  const recentRuns = useMemo(
    () => [...(runs ?? [])].sort((a, b) => b.created_at.localeCompare(a.created_at)).slice(0, 5),
    [runs],
  );

  const completionRate = useMemo(() => {
    const allRuns = runs ?? [];
    if (allRuns.length === 0) return null;
    const completed = allRuns.filter((r) => r.status === 'completed').length;
    const terminal = allRuns.filter(
      (r) => r.status === 'completed' || r.status === 'failed' || r.status === 'cancelled',
    ).length;
    if (terminal === 0) return null;
    return Math.round((completed / terminal) * 100);
  }, [runs]);

  const error = sessionsError || machinesError;
  const isLoading = sessionsLoading || machinesLoading;
  const isJobsRunsLoading = jobsLoading || runsLoading;

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

      {/* Stats Row — Sessions & Machines */}
      <div className="grid grid-cols-2 sm:grid-cols-3 gap-4">
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

      {/* Stats Row — Jobs & Runs */}
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-4">
        <StatCard
          icon={<Workflow size={24} />}
          label="Total Jobs"
          value={isJobsRunsLoading ? '--' : String(jobs?.length ?? 0)}
          href="/jobs"
        />
        <StatCard
          icon={<Play size={24} />}
          label="Total Runs"
          value={isJobsRunsLoading ? '--' : String(runs?.length ?? 0)}
          href="/runs"
        />
        <StatCard
          icon={<Activity size={24} />}
          label="Completion Rate"
          value={isJobsRunsLoading ? '--' : completionRate !== null ? `${completionRate}%` : 'N/A'}
        />
        <StatCard
          icon={<Clock size={24} />}
          label="Active Schedules"
          value={isJobsRunsLoading ? '--' : String(countJobsWithSchedules(jobs ?? []))}
          href="/jobs"
        />
      </div>

      {/* Recent Jobs & Recent Runs — 2-column */}
      <div className="grid grid-cols-1 xl:grid-cols-2 gap-6">
        {/* Recent Jobs */}
        <section>
          <div className="flex items-center justify-between mb-3">
            <h2 className="text-sm font-medium text-text-secondary uppercase tracking-wider">
              Recent Jobs
            </h2>
            <Link
              to="/jobs"
              className="text-xs text-accent-primary hover:text-accent-primary/80 transition-colors"
            >
              View All Jobs
            </Link>
          </div>
          {isJobsRunsLoading ? (
            <SkeletonList count={3} />
          ) : recentJobs.length === 0 ? (
            <div className="bg-bg-secondary rounded-lg p-6 text-center">
              <p className="text-sm text-text-secondary">No jobs yet.</p>
              <Link
                to="/jobs/new"
                className="inline-block mt-2 text-xs text-accent-primary hover:text-accent-primary/80"
              >
                Create your first job
              </Link>
            </div>
          ) : (
            <div className="bg-bg-secondary rounded-lg divide-y divide-border-primary">
              {recentJobs.map((job) => (
                <RecentJobRow key={job.job_id} job={job} />
              ))}
            </div>
          )}
        </section>

        {/* Recent Runs */}
        <section>
          <div className="flex items-center justify-between mb-3">
            <h2 className="text-sm font-medium text-text-secondary uppercase tracking-wider">
              Recent Runs
            </h2>
            <Link
              to="/runs"
              className="text-xs text-accent-primary hover:text-accent-primary/80 transition-colors"
            >
              View All Runs
            </Link>
          </div>
          {isJobsRunsLoading ? (
            <SkeletonList count={3} />
          ) : recentRuns.length === 0 ? (
            <div className="bg-bg-secondary rounded-lg p-6 text-center">
              <p className="text-sm text-text-secondary">No runs yet.</p>
            </div>
          ) : (
            <div className="bg-bg-secondary rounded-lg divide-y divide-border-primary">
              {recentRuns.map((run) => (
                <RecentRunRow key={run.run_id} run={run} />
              ))}
            </div>
          )}
        </section>
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

/**
 * Count jobs that have ever been run (as a proxy for "has schedules").
 * Since there is no global schedules endpoint, we use last_run_status presence
 * as a heuristic. This is intentionally conservative.
 */
function countJobsWithSchedules(jobs: Job[]): number {
  // We can't know schedule count without per-job fetches.
  // Return the number of jobs that have a last_run_status, which implies
  // they were triggered at least once (by schedule or manually).
  return jobs.filter((j) => j.last_run_status !== undefined && j.last_run_status !== null).length;
}

function StatCard({
  icon,
  label,
  value,
  href,
}: {
  icon: React.ReactNode;
  label: string;
  value: string;
  href?: string;
}) {
  const content = (
    <div className="bg-bg-secondary rounded-lg p-4 flex items-center gap-4">
      <div className="text-accent-primary shrink-0">{icon}</div>
      <div>
        <p className="text-2xl font-bold text-text-primary">{value}</p>
        <p className="text-xs text-text-secondary">{label}</p>
      </div>
    </div>
  );

  if (href) {
    return (
      <Link to={href} className="block hover:opacity-80 transition-opacity">
        {content}
      </Link>
    );
  }
  return content;
}

function RecentJobRow({ job }: { job: Job }) {
  return (
    <Link
      to={`/jobs/${job.job_id}`}
      className="flex items-center justify-between px-4 py-3 hover:bg-bg-tertiary transition-colors first:rounded-t-lg last:rounded-b-lg"
    >
      <div className="flex-1 min-w-0 mr-3">
        <p className="text-sm font-medium text-text-primary truncate">{job.name}</p>
        <p className="text-xs text-text-secondary mt-0.5">
          {job.step_count != null ? `${job.step_count} step${job.step_count !== 1 ? 's' : ''}` : 'No steps'}
          {' · '}
          <TimeAgo date={job.updated_at} />
        </p>
      </div>
      {job.last_run_status && (
        <StatusBadge status={job.last_run_status} size="sm" />
      )}
    </Link>
  );
}

function RecentRunRow({ run }: { run: Run }) {
  const date = run.started_at ?? run.created_at;
  return (
    <Link
      to={`/runs/${run.run_id}`}
      className="flex items-center justify-between px-4 py-3 hover:bg-bg-tertiary transition-colors first:rounded-t-lg last:rounded-b-lg"
    >
      <div className="flex-1 min-w-0 mr-3">
        <p className="text-sm font-medium text-text-primary truncate">
          {run.job_name ?? run.job_id}
        </p>
        <p className="text-xs text-text-secondary mt-0.5">
          {run.trigger_type ? (
            <span className="capitalize">{run.trigger_type}</span>
          ) : (
            'manual'
          )}
          {' · '}
          <TimeAgo date={date} />
        </p>
      </div>
      <StatusBadge status={run.status} size="sm" />
    </Link>
  );
}

function SkeletonList({ count }: { count: number }) {
  return (
    <div className="bg-bg-secondary rounded-lg divide-y divide-border-primary">
      {Array.from({ length: count }, (_, i) => (
        <div key={i} className="px-4 py-3 flex items-center gap-3 animate-pulse">
          <div className="flex-1">
            <div className="h-4 bg-bg-tertiary rounded w-1/2 mb-2" />
            <div className="h-3 bg-bg-tertiary rounded w-1/3" />
          </div>
          <div className="h-4 bg-bg-tertiary rounded w-16" />
        </div>
      ))}
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
