import { useState, useEffect, useMemo } from 'react';
import { createPortal } from 'react-dom';
import { useNavigate } from 'react-router';
import { toast } from 'sonner';
import { useCreateSession } from '../../hooks/useSessions.ts';
import { useMachines } from '../../hooks/useMachines.ts';
import { TemplatePicker } from '../templates/TemplatePicker.tsx';
import type { SessionTemplate } from '../../types/template.ts';

// Estimated terminal font metrics and layout offsets.
// These must match the configuration used by the xterm.js Terminal
// (e.g. fontSize 14, JetBrains Mono) and the surrounding UI chrome.
const TERMINAL_FONT_CHAR_WIDTH_PX = 8.4;
const TERMINAL_FONT_LINE_HEIGHT_PX = 18;
const TERMINAL_SIDEBAR_WIDTH_PX = 56;   // w-14 = 3.5rem = 56px
const TERMINAL_CHROME_HEIGHT_PX = 120;  // top bar + status bar + terminal status bar + padding

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
  const [selectedTemplate, setSelectedTemplate] = useState<SessionTemplate | null>(null);
  const [variables, setVariables] = useState<Record<string, string>>({});

  const templatePrompt = selectedTemplate?.initial_prompt ?? '';
  const variableNames = useMemo(() => {
    if (!templatePrompt) return [];
    const matches = templatePrompt.matchAll(/\$\{([A-Z][A-Z0-9_]*)\}/g);
    const unique = new Set<string>();
    for (const match of matches) {
      unique.add(match[1]);
    }
    return [...unique];
  }, [templatePrompt]);

  useEffect(() => {
    if (preselectedMachineId) {
      // eslint-disable-next-line react-hooks/set-state-in-effect -- syncing prop to local form state
      setMachineId(preselectedMachineId);
    }
  }, [preselectedMachineId]);

  useEffect(() => {
    if (!open) {
      // eslint-disable-next-line react-hooks/set-state-in-effect -- resetting form on close
      if (!preselectedMachineId) setMachineId('');
      setWorkingDir('');
      setCommand('');
      setSelectedTemplate(null);
      setVariables({});
    }
  }, [open, preselectedMachineId]);

  const onlineMachines = machines?.filter((m) => m.status === 'connected') ?? [];

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!machineId) return;

    try {
      // Estimate terminal dimensions from the viewport so the PTY starts
      // at the right size instead of the 80x24 default. Uses the same font
      // metrics as the xterm.js Terminal.
      const cols = Math.max(
        80,
        Math.floor(
          (window.innerWidth - TERMINAL_SIDEBAR_WIDTH_PX - 32) / TERMINAL_FONT_CHAR_WIDTH_PX,
        ),
      );
      const rows = Math.max(
        24,
        Math.floor(
          (window.innerHeight - TERMINAL_CHROME_HEIGHT_PX) / TERMINAL_FONT_LINE_HEIGHT_PX,
        ),
      );

      const session = await createSession.mutateAsync({
        machine_id: machineId,
        terminal_size: { cols, rows },
        ...(command ? { command } : {}),
        ...(workingDir ? { working_dir: workingDir } : {}),
        ...(selectedTemplate ? { template_id: selectedTemplate.template_id } : {}),
        ...(variableNames.length > 0 ? { variables } : {}),
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
      <div className="relative bg-bg-secondary border border-border-primary rounded-lg shadow-xl max-w-md w-full mx-4 p-6">
        <h2 className="text-lg font-semibold text-text-primary mb-4">New Session</h2>

        <form onSubmit={handleSubmit} className="flex flex-col gap-4">
          <div>
            <label className="block text-sm text-text-secondary mb-1">Template</label>
            <TemplatePicker
              onSelect={(template) => {
                setSelectedTemplate(template);
                setVariables({});
                if (template.command) setCommand(template.command);
                if (template.working_dir) setWorkingDir(template.working_dir);
              }}
            />
            {selectedTemplate && (
              <p className="text-xs text-accent-primary mt-1">
                Using template: {selectedTemplate.name}
              </p>
            )}
          </div>

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

          {variableNames.length > 0 && variableNames.map((varName) => (
            <div key={varName}>
              <label className="block text-sm text-text-secondary mb-1">{varName}</label>
              <input
                type="text"
                value={variables[varName] ?? ''}
                onChange={(e) => setVariables((prev) => ({ ...prev, [varName]: e.target.value }))}
                placeholder={`Enter value for ${varName}`}
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
              {createSession.isPending ? 'Creating...' : 'Create Session'}
            </button>
          </div>
        </form>
      </div>
    </div>,
    document.body,
  );
}
