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
    <footer
      className={`flex items-center justify-between h-6 px-4 text-xs font-medium shrink-0 transition-colors ${
        connected
          ? 'bg-accent-primary text-white'
          : 'bg-status-error text-white'
      }`}
    >
      <div className="flex items-center gap-4">
        <span className="flex items-center gap-1.5">
          <span
            className={`w-1.5 h-1.5 rounded-full ${
              connected ? 'bg-white animate-pulse' : 'bg-white/60'
            }`}
          />
          {connected ? 'Connected' : 'Disconnected'}
        </span>
      </div>

      <div className="flex items-center gap-4 opacity-90">
        <span>{machineCount} machine{machineCount !== 1 ? 's' : ''}</span>
        <span>{sessionCount} active session{sessionCount !== 1 ? 's' : ''}</span>
      </div>
    </footer>
  );
}
