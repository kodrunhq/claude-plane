import { Terminal } from 'lucide-react';
import { SessionCard } from './SessionCard.tsx';
import { EmptyState } from '../shared/EmptyState.tsx';
import type { Session } from '../../types/session.ts';

interface SessionListProps {
  sessions: Session[];
  onAttach: (id: string) => void;
  onTerminate: (id: string) => void;
  emptyMessage?: string;
}

export function SessionList({ sessions, onAttach, onTerminate, emptyMessage }: SessionListProps) {
  if (sessions.length === 0) {
    return (
      <EmptyState
        icon={<Terminal size={48} />}
        title="No sessions"
        description={emptyMessage ?? 'No sessions found. Create a new session to get started.'}
      />
    );
  }

  return (
    <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
      {sessions.map((session) => (
        <SessionCard
          key={session.session_id}
          session={session}
          onAttach={onAttach}
          onTerminate={onTerminate}
        />
      ))}
    </div>
  );
}
