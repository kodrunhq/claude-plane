import { useEventStream } from '../../hooks/useEventStream.ts';
import { useMachines } from '../../hooks/useMachines.ts';
import { useSessions } from '../../hooks/useSessions.ts';

export function StatusBar() {
  const { connected } = useEventStream();
  const { data: machines } = useMachines();
  const { data: sessions } = useSessions();

  const machineCount = machines?.length ?? 0;
  const sessionCount = sessions?.filter((s) => s.status === 'running' || s.status === 'created').length ?? 0;

  return (
    <footer className="flex items-center justify-between h-6 px-4 bg-bg-secondary border-t border-gray-700 text-xs text-text-secondary shrink-0">
      <div className="flex items-center gap-4">
        <span className="flex items-center gap-1.5">
          <span className={`w-2 h-2 rounded-full ${connected ? 'bg-status-success' : 'bg-status-error'}`} />
          {connected ? 'Connected' : 'Disconnected'}
        </span>
      </div>

      <div className="flex items-center gap-4">
        <span>{machineCount} machine{machineCount !== 1 ? 's' : ''}</span>
        <span>{sessionCount} active session{sessionCount !== 1 ? 's' : ''}</span>
      </div>
    </footer>
  );
}
