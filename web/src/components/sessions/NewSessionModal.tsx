import { useState, useEffect } from 'react';
import { createPortal } from 'react-dom';
import { useNavigate } from 'react-router';
import { toast } from 'sonner';
import { useCreateSession } from '../../hooks/useSessions.ts';
import { useMachines } from '../../hooks/useMachines.ts';

interface NewSessionModalProps {
  open: boolean;
  onClose: () => void;
  preselectedMachineId?: string;
}

export function NewSessionModal({ open, onClose, preselectedMachineId }: NewSessionModalProps) {
  const navigate = useNavigate();
  const createSession = useCreateSession();
  const { data: machines } = useMachines();

  const [machineId, setMachineId] = useState(preselectedMachineId ?? '');
  const [workingDir, setWorkingDir] = useState('');
  const [command, setCommand] = useState('');

  useEffect(() => {
    if (preselectedMachineId) {
      setMachineId(preselectedMachineId);
    }
  }, [preselectedMachineId]);

  useEffect(() => {
    if (!open) {
      // Reset form when closing
      if (!preselectedMachineId) setMachineId('');
      setWorkingDir('');
      setCommand('');
    }
  }, [open, preselectedMachineId]);

  const onlineMachines = machines?.filter((m) => m.status === 'connected') ?? [];

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!machineId) return;

    try {
      const session = await createSession.mutateAsync({
        machine_id: machineId,
        ...(command ? { command } : {}),
        ...(workingDir ? { working_dir: workingDir } : {}),
      });
      toast.success('Session created');
      onClose();
      navigate(`/sessions/${session.session_id}`);
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to create session';
      toast.error(message);
    }
  }

  if (!open) return null;

  return createPortal(
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      <div className="absolute inset-0 bg-black/60" onClick={onClose} />
      <div className="relative bg-bg-secondary border border-gray-700 rounded-lg shadow-xl max-w-md w-full mx-4 p-6">
        <h2 className="text-lg font-semibold text-text-primary mb-4">New Session</h2>

        <form onSubmit={handleSubmit} className="flex flex-col gap-4">
          <div>
            <label className="block text-sm text-text-secondary mb-1">Machine</label>
            <select
              value={machineId}
              onChange={(e) => setMachineId(e.target.value)}
              className="w-full rounded-md bg-bg-tertiary border border-gray-600 text-text-primary text-sm px-3 py-2 focus:outline-none focus:ring-1 focus:ring-accent-primary"
              required
            >
              <option value="">Select a machine...</option>
              {onlineMachines.map((m) => (
                <option key={m.machine_id} value={m.machine_id}>
                  {m.display_name || m.machine_id} ({m.machine_id.slice(0, 8)})
                </option>
              ))}
            </select>
            {onlineMachines.length === 0 && (
              <p className="text-xs text-status-warning mt-1">No online machines available</p>
            )}
          </div>

          <div>
            <label className="block text-sm text-text-secondary mb-1">
              Working Directory <span className="text-text-secondary/50">(optional)</span>
            </label>
            <input
              type="text"
              value={workingDir}
              onChange={(e) => setWorkingDir(e.target.value)}
              placeholder="/home/user/project"
              className="w-full rounded-md bg-bg-tertiary border border-gray-600 text-text-primary text-sm px-3 py-2 focus:outline-none focus:ring-1 focus:ring-accent-primary placeholder:text-text-secondary/30"
            />
          </div>

          <div>
            <label className="block text-sm text-text-secondary mb-1">
              Command <span className="text-text-secondary/50">(defaults to "claude")</span>
            </label>
            <input
              type="text"
              value={command}
              onChange={(e) => setCommand(e.target.value)}
              placeholder="claude"
              className="w-full rounded-md bg-bg-tertiary border border-gray-600 text-text-primary text-sm px-3 py-2 focus:outline-none focus:ring-1 focus:ring-accent-primary placeholder:text-text-secondary/30"
            />
          </div>

          <div className="flex justify-end gap-3 mt-2">
            <button
              type="button"
              onClick={onClose}
              className="px-4 py-2 text-sm rounded-md text-text-secondary hover:text-text-primary bg-bg-tertiary hover:bg-bg-tertiary/80 transition-colors"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={!machineId || createSession.isPending}
              className="px-4 py-2 text-sm rounded-md bg-accent-primary hover:bg-accent-primary/80 text-white transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {createSession.isPending ? 'Creating...' : 'Create Session'}
            </button>
          </div>
        </form>
      </div>
    </div>,
    document.body,
  );
}
