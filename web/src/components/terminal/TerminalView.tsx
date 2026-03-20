import { useRef, useEffect } from 'react';
import { ArrowDown } from 'lucide-react';
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
  ended: 'Session ended',
  disconnected: 'Disconnected',
  agent_offline: 'Agent offline',
};

const statusColors: Record<TerminalStatus, string> = {
  connecting: 'text-yellow-400',
  replaying: 'text-blue-400',
  live: 'text-green-400',
  ended: 'text-text-secondary',
  disconnected: 'text-red-400',
  agent_offline: 'text-orange-400',
};

export function TerminalView({ sessionId, onStatusChange, className = '', useWebGL, fontSize }: TerminalViewProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const { status, showScrollButton, focusTerminal, scrollToBottom } = useTerminalSession(sessionId, containerRef, { useWebGL, fontSize });

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
                  : status === 'ended'
                    ? 'bg-gray-400'
                    : status === 'agent_offline'
                      ? 'bg-orange-400'
                      : 'bg-red-400'
            }`}
          />
          <span className={statusColors[status]}>{statusLabels[status]}</span>
        </div>
        <span className="text-text-secondary font-mono">{sessionId.slice(0, 8)}</span>
      </div>

      {/* Agent offline overlay */}
      {status === 'agent_offline' && (
        <div className="px-4 py-2 bg-orange-900/20 border-b border-orange-600/30 text-xs text-orange-400">
          The agent that ran this session is offline. Session replay is unavailable until it reconnects.
        </div>
      )}

      {/* Terminal container */}
      <div
        className="flex-1 min-h-0 relative"
        style={{ backgroundColor: '#1a1b26' }}
      >
        <div
          ref={containerRef}
          className="absolute inset-0"
          onClick={focusTerminal}
        />
        {showScrollButton && (
          <button
            onClick={(e) => { e.stopPropagation(); scrollToBottom(); }}
            className="absolute bottom-3 right-3 z-10 flex items-center gap-1.5 px-2.5 py-1.5 text-xs rounded-md bg-bg-secondary/90 border border-border-primary text-text-secondary hover:text-text-primary hover:bg-bg-tertiary transition-colors backdrop-blur-sm"
            title="Scroll to bottom"
          >
            <ArrowDown size={14} />
            <span>Bottom</span>
          </button>
        )}
      </div>
    </div>
  );
}
