import { useRef, useEffect } from 'react';
import { useTerminalSession } from '../../hooks/useTerminalSession.ts';
import type { TerminalStatus } from '../../types/session.ts';

interface TerminalViewProps {
  sessionId: string;
  onStatusChange?: (status: TerminalStatus) => void;
  className?: string;
  useWebGL?: boolean;
}

const statusLabels: Record<TerminalStatus, string> = {
  connecting: 'Connecting...',
  replaying: 'Loading history...',
  live: 'Connected',
  disconnected: 'Disconnected',
};

const statusColors: Record<TerminalStatus, string> = {
  connecting: 'text-yellow-400',
  replaying: 'text-blue-400',
  live: 'text-green-400',
  disconnected: 'text-red-400',
};

export function TerminalView({ sessionId, onStatusChange, className = '', useWebGL }: TerminalViewProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const { status, fitTerminal } = useTerminalSession(sessionId, containerRef, { useWebGL });

  // Notify parent of status changes
  useEffect(() => {
    onStatusChange?.(status);
  }, [status, onStatusChange]);

  // Re-fit on mount with staggered timing to handle deferred rendering.
  // The hook already does staggered fits, but TerminalView may mount after
  // the hook's initial fits have already fired (e.g., when switching tabs).
  useEffect(() => {
    const timers = [
      setTimeout(() => fitTerminal(), 50),
      setTimeout(() => fitTerminal(), 250),
    ];
    return () => timers.forEach(clearTimeout);
  }, [fitTerminal]);

  return (
    <div className={`flex flex-col h-full ${className}`}>
      {/* Status bar */}
      <div className="flex items-center justify-between px-3 py-1.5 bg-bg-secondary border-b border-border-primary text-xs">
        <div className="flex items-center gap-2">
          <span
            className={`inline-block w-2 h-2 rounded-full ${
              status === 'live'
                ? 'bg-green-400'
                : status === 'connecting' || status === 'replaying'
                  ? 'bg-yellow-400 animate-pulse'
                  : 'bg-red-400'
            }`}
          />
          <span className={statusColors[status]}>{statusLabels[status]}</span>
        </div>
        <span className="text-text-secondary font-mono">{sessionId.slice(0, 8)}</span>
      </div>

      {/* Terminal container */}
      <div
        ref={containerRef}
        className="flex-1 min-h-0"
        style={{ backgroundColor: '#1a1b26' }}
      />
    </div>
  );
}
