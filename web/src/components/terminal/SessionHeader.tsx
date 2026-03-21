import { useState } from 'react';
import { Link } from 'react-router';
import { ArrowLeft, Monitor, FolderOpen, Square } from 'lucide-react';
import { StatusBadge } from '../shared/StatusBadge.tsx';
import { ConfirmDialog } from '../shared/ConfirmDialog.tsx';
import { Breadcrumb } from '../shared/Breadcrumb.tsx';
import { CopyableId } from '../shared/CopyableId.tsx';
import type { Session } from '../../types/session.ts';

interface SessionHeaderProps {
  session: Session | undefined;
  isLoading: boolean;
  onTerminate: (id: string) => void;
}

export function SessionHeader({ session, isLoading, onTerminate }: SessionHeaderProps) {
  const [confirmOpen, setConfirmOpen] = useState(false);

  const isActive = session?.status === 'running' || session?.status === 'created' || session?.status === 'waiting_for_input';

  function handleConfirmTerminate() {
    if (!session) return;
    onTerminate(session.session_id);
    setConfirmOpen(false);
  }

  return (
    <>
      <div className="px-3 pt-2 pb-1 bg-bg-secondary">
        <Breadcrumb items={[
          { label: 'Sessions', to: '/sessions' },
          { label: session ? `Session ${session.session_id.slice(0, 8)}` : 'Session' },
        ]} />
      </div>

      <div className="flex flex-wrap items-center gap-x-4 gap-y-2 px-3 py-2 bg-bg-secondary border-b border-border-primary text-xs">
        {/* Back link */}
        <Link
          to="/sessions"
          className="inline-flex items-center gap-1 text-text-secondary hover:text-text-primary transition-colors shrink-0"
          aria-label="Back to sessions"
        >
          <ArrowLeft size={14} />
          <span className="hidden sm:inline">Sessions</span>
        </Link>

        {/* Separator */}
        <span className="hidden sm:block w-px h-4 bg-border-primary" />

        {isLoading ? (
          <span className="text-text-secondary animate-pulse">Loading session...</span>
        ) : session ? (
          <>
            {/* Session ID */}
            <CopyableId id={session.session_id} className="text-xs opacity-60 shrink-0" />

            {/* Status */}
            <StatusBadge status={session.status} size="sm" />

            {/* Command */}
            <span className="font-mono text-text-primary truncate max-w-[200px]" title={session.command}>
              {session.command || 'claude'}
            </span>

            {/* Machine */}
            <span
              className="inline-flex items-center gap-1 text-text-secondary shrink-0"
              title={session.machine_id}
            >
              <Monitor size={12} className="opacity-60" />
              <span className="font-mono truncate max-w-[160px]">
                {session.machine_id}
              </span>
            </span>

            {/* Model badge */}
            {session.model && (
              <span className="inline-flex items-center px-1.5 py-0.5 text-xs font-medium rounded bg-purple-500/10 text-purple-400 shrink-0">
                {session.model}
              </span>
            )}

            {/* Working directory */}
            {session.working_dir && (
              <span
                className="hidden md:inline-flex items-center gap-1 text-text-secondary shrink-0"
                title={session.working_dir}
              >
                <FolderOpen size={12} className="opacity-60" />
                <span className="font-mono truncate max-w-[180px]">
                  {session.working_dir}
                </span>
              </span>
            )}

            {/* Spacer */}
            <span className="flex-1" />

            {/* Terminate button */}
            {isActive && (
              <button
                type="button"
                aria-label="Terminate session"
                className="inline-flex items-center gap-1 px-2 py-1 rounded text-xs font-medium bg-status-error/10 text-status-error hover:bg-status-error/20 transition-all shrink-0"
                onClick={() => setConfirmOpen(true)}
              >
                <Square size={12} />
                <span className="hidden sm:inline">Terminate</span>
              </button>
            )}
          </>
        ) : (
          <span className="text-text-secondary">Session not found</span>
        )}
      </div>

      <ConfirmDialog
        open={confirmOpen}
        title="Terminate Session"
        message="Are you sure you want to terminate this session? This action cannot be undone."
        confirmLabel="Terminate"
        variant="danger"
        onConfirm={handleConfirmTerminate}
        onCancel={() => setConfirmOpen(false)}
      />
    </>
  );
}
