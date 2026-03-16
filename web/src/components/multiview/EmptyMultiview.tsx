import { LayoutGrid } from 'lucide-react';
import { useNavigate } from 'react-router';

export function EmptyMultiview() {
  const navigate = useNavigate();

  return (
    <div className="flex flex-col items-center justify-center h-full text-center px-4">
      <LayoutGrid size={48} className="text-text-secondary mb-4" strokeWidth={1} />
      <h2 className="text-lg font-semibold text-text-primary mb-2">Multi-View</h2>
      <p className="text-sm text-text-secondary mb-6 max-w-md">
        View and interact with multiple terminal sessions simultaneously.
        Select sessions from the sessions page or create a new workspace.
      </p>
      <button
        onClick={() => navigate('/sessions')}
        className="px-4 py-2 text-sm rounded bg-bg-secondary border border-border-primary text-text-primary hover:bg-bg-tertiary transition-colors"
      >
        Go to Sessions
      </button>
    </div>
  );
}
