import { useState, useEffect, useMemo } from 'react';
import { createPortal } from 'react-dom';
import { useNavigate } from 'react-router';
import { toast } from 'sonner';
import { useCreateSession } from '../../hooks/useSessions.ts';
import { useMachines } from '../../hooks/useMachines.ts';
import { extractTemplateVariables } from '../../lib/templateVars.ts';
import type { SessionTemplate } from '../../types/template.ts';

interface LaunchTemplateModalProps {
  open: boolean;
  onClose: () => void;
  template: SessionTemplate | null;
}

export function LaunchTemplateModal({ open, onClose, template }: LaunchTemplateModalProps) {
  const navigate = useNavigate();
  const createSession = useCreateSession();
  const { data: machines } = useMachines();

  const [machineId, setMachineId] = useState('');
  const [variables, setVariables] = useState<Record<string, string>>({});

  const templatePrompt = template?.initial_prompt ?? '';
  const variableNames = useMemo(
    () => (templatePrompt ? extractTemplateVariables(templatePrompt) : []),
    [templatePrompt],
  );

  const onlineMachines = useMemo(
    () => (machines ?? []).filter((m) => m.status === 'connected'),
    [machines],
  );

  useEffect(() => {
    if (!open) {
      // eslint-disable-next-line react-hooks/set-state-in-effect -- resetting form on close
      setMachineId('');
      setVariables({});
    }
  }, [open]);

  function handleVariableChange(name: string, value: string) {
    setVariables((prev) => ({ ...prev, [name]: value }));
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!machineId || !template) return;

    try {
      const session = await createSession.mutateAsync({
        machine_id: machineId,
        template_id: template.template_id,
        variables: variableNames.length > 0 ? variables : undefined,
      });
      toast.success('Session created from template');
      onClose();
      navigate(`/sessions/${session.session_id}`);
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to create session';
      toast.error(message);
    }
  }

  if (!open || !template) return null;

  return createPortal(
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      <div className="absolute inset-0 bg-black/60" onClick={onClose} />
      <div className="relative bg-bg-secondary border border-border-primary rounded-lg shadow-xl max-w-md w-full mx-4 p-6">
        <h2 className="text-lg font-semibold text-text-primary mb-1">
          Launch Template
        </h2>
        <p className="text-sm text-text-secondary mb-4">{template.name}</p>

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
                  {m.display_name || m.machine_id}
                </option>
              ))}
            </select>
            {onlineMachines.length === 0 && (
              <p className="text-xs text-status-warning mt-1">No online machines available</p>
            )}
          </div>

          {variableNames.map((name) => (
            <div key={name}>
              <label className="block text-sm text-text-secondary mb-1">{name}</label>
              <input
                type="text"
                value={variables[name] ?? ''}
                onChange={(e) => handleVariableChange(name, e.target.value)}
                placeholder={`Enter value for ${name}`}
                className="w-full rounded-md bg-bg-tertiary border border-gray-600 text-text-primary text-sm px-3 py-2 focus:outline-none focus:ring-1 focus:ring-accent-primary placeholder:text-text-secondary/30"
              />
            </div>
          ))}

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
              {createSession.isPending ? 'Launching...' : 'Launch'}
            </button>
          </div>
        </form>
      </div>
    </div>,
    document.body,
  );
}
