import { useMemo } from 'react';
import { useNavigate } from 'react-router';
import { TerminalView } from '../terminal/TerminalView';
import { PaneHeader } from './PaneHeader';
import { PaneEmptyState } from './PaneEmptyState';
import { useSession } from '../../hooks/useSessions';
import { useMachines } from '../../hooks/useMachines';
import { useUIPrefs } from '../../hooks/useUIPrefs';
import type { Pane } from '../../types/multiview';

interface TerminalPaneProps {
  readonly pane: Pane;
  readonly isFocused: boolean;
  readonly isMaximized: boolean;
  readonly useWebGL: boolean;
  readonly onFocus: () => void;
  readonly onMaximize: () => void;
  readonly onPickSession: () => void;
  readonly onRemovePane?: () => void;
  readonly canRemove?: boolean;
}

function detectSessionType(command: string): 'claude' | 'terminal' {
  return command.toLowerCase().includes('claude') ? 'claude' : 'terminal';
}

export function TerminalPane({
  pane,
  isFocused,
  isMaximized,
  useWebGL,
  onFocus,
  onMaximize,
  onPickSession,
  onRemovePane,
  canRemove,
}: TerminalPaneProps) {
  const navigate = useNavigate();
  const hasSession = pane.sessionId !== '';
  const { data: session } = useSession(hasSession ? pane.sessionId : '');
  const { data: machines } = useMachines();
  const { terminal_font_size } = useUIPrefs();

  const machineName = useMemo(() => {
    if (!session || !machines) return '...';
    const machine = machines.find((m) => m.machine_id === session.machine_id);
    if (machine?.display_name) return machine.display_name;
    // Show full machine_id (or a meaningful prefix) so it's distinguishable from session IDs
    return machine?.machine_id ?? session.machine_id ?? 'unknown';
  }, [session, machines]);

  const isStale = session && ['completed', 'failed', 'terminated'].includes(session.status);

  const borderClass = isFocused
    ? 'ring-2 ring-accent-primary shadow-[0_0_8px_rgba(99,102,241,0.3)]'
    : 'ring-1 ring-border-primary';

  return (
    <div
      className={`flex flex-col h-full rounded overflow-hidden ${borderClass} transition-shadow`}
      onClick={onFocus}
    >
      {session && (
        <PaneHeader
          sessionType={detectSessionType(session.command)}
          machineName={machineName}
          sessionId={pane.sessionId}
          workingDir={session.working_dir}
          isMaximized={isMaximized}
          onMaximize={onMaximize}
          onSwapSession={onPickSession}
          onRemovePane={onRemovePane}
          onOpenFullView={() => navigate(`/sessions/${pane.sessionId}`)}
          canRemove={canRemove}
        />
      )}
      <div className="flex-1 min-h-0 relative">
        {!hasSession ? (
          <PaneEmptyState onPickSession={onPickSession} />
        ) : !session ? (
          <PaneEmptyState
            message="Session no longer available"
            onPickSession={onPickSession}
          />
        ) : isStale ? (
          <div className="relative h-full">
            <TerminalView
              sessionId={pane.sessionId}
              className="h-full opacity-50"
              useWebGL={useWebGL}
              fontSize={terminal_font_size}
            />
            <div className="absolute inset-0 flex items-center justify-center bg-black/40">
              <div className="text-center">
                <p className="text-text-secondary text-sm mb-2">Session ended</p>
                <button
                  onClick={(e) => {
                    e.stopPropagation();
                    onPickSession();
                  }}
                  className="px-3 py-1.5 text-xs rounded bg-bg-secondary text-text-primary hover:bg-bg-tertiary transition-colors"
                >
                  Swap session
                </button>
              </div>
            </div>
          </div>
        ) : (
          <TerminalView
            sessionId={pane.sessionId}
            className="h-full"
            useWebGL={useWebGL}
            fontSize={terminal_font_size}
          />
        )}
      </div>
    </div>
  );
}
