import { LayoutGrid, Plus } from 'lucide-react';
import { useNavigate } from 'react-router';

interface EmptyMultiviewProps {
  readonly onCreateWorkspace?: () => void;
}

export function EmptyMultiview({ onCreateWorkspace }: EmptyMultiviewProps) {
  const navigate = useNavigate();

  return (
    <div className="flex flex-col items-center justify-center h-full text-center px-4">
      <LayoutGrid size={48} className="text-text-secondary mb-4" strokeWidth={1} />
      <h2 className="text-lg font-semibold text-text-primary mb-2">Multi-View</h2>
      <p className="text-sm text-text-secondary mb-6 max-w-md">
        View and interact with multiple terminal sessions simultaneously.
        Pick sessions to create a workspace, or go to the sessions page.
      </p>
      <div className="flex items-center gap-3">
        {onCreateWorkspace && (
          <button
            onClick={onCreateWorkspace}
            className="flex items-center gap-2 px-4 py-2 text-sm rounded bg-accent-primary text-white hover:bg-accent-primary/80 transition-colors"
          >
            <Plus size={16} />
            New Workspace
          </button>
        )}
        <button
          onClick={() => navigate('/sessions')}
          className="px-4 py-2 text-sm rounded bg-bg-secondary border border-border-primary text-text-primary hover:bg-bg-tertiary transition-colors"
        >
          Go to Sessions
        </button>
      </div>
    </div>
  );
}
