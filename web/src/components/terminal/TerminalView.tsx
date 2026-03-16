import { useRef, useEffect } from 'react';
import { useTerminalSession } from '../../hooks/useTerminalSession.ts';
import type { TerminalStatus } from '../../types/session.ts';

interface TerminalViewProps {
  sessionId: string;
  onStatusChange?: (status: TerminalStatus) => void;
  className?: string;
  useWebGL?: boolean;
  fontSize?: number;
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

export function TerminalView({ sessionId, onStatusChange, className = '', useWebGL, fontSize }: TerminalViewProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const { status, focusTerminal } = useTerminalSession(sessionId, containerRef, { useWebGL, fontSize });

  useEffect(() => {
    onStatusChange?.(status);
  }, [status, onStatusChange]);

  // Auto-focus terminal when it becomes live so keystrokes work immediately.
  useEffect(() => {
    if (status === 'live') {
      focusTerminal();
    }
  }, [status, focusTerminal]);

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
        onClick={focusTerminal}
      />
    </div>
  );
}
