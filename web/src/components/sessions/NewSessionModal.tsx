import { useState, useEffect, useMemo } from 'react';
import { createPortal } from 'react-dom';
import { useNavigate } from 'react-router';
import { toast } from 'sonner';
import { Bot, Terminal, FolderOpen } from 'lucide-react';
import { useCreateSession } from '../../hooks/useSessions.ts';
import { useMachines } from '../../hooks/useMachines.ts';
import { extractTemplateVariables } from '../../lib/templateVars.ts';
import { TemplatePicker } from '../templates/TemplatePicker.tsx';
import { DirectoryBrowserModal } from './DirectoryBrowserModal.tsx';
import type { SessionTemplate } from '../../types/template.ts';

type SessionType = 'claude' | 'terminal';

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

  const [sessionType, setSessionType] = useState<SessionType>('claude');
  const [machineId, setMachineId] = useState(preselectedMachineId ?? '');
  const [workingDir, setWorkingDir] = useState('');
  const [workingDirDirty, setWorkingDirDirty] = useState(false);
  const [additionalArgs, setAdditionalArgs] = useState('');
  const [model, setModel] = useState('');
  const [skipPermissions, setSkipPermissions] = useState('');
  const [selectedTemplate, setSelectedTemplate] = useState<SessionTemplate | null>(null);
  const [variables, setVariables] = useState<Record<string, string>>({});
  const [browseOpen, setBrowseOpen] = useState(false);

  const templatePrompt = selectedTemplate?.initial_prompt ?? '';
  const variableNames = useMemo(
    () => (templatePrompt ? extractTemplateVariables(templatePrompt) : []),
    [templatePrompt],
  );

  useEffect(() => {
    if (preselectedMachineId) {
      // eslint-disable-next-line react-hooks/set-state-in-effect -- syncing prop to local form state
      setMachineId(preselectedMachineId);
    }
  }, [preselectedMachineId]);

  // Pre-fill working directory from machine's home_dir when machine changes
  useEffect(() => {
    if (!workingDirDirty && machineId && machines) {
      const machine = machines.find((m) => m.machine_id === machineId);
      if (machine?.home_dir) {
        // eslint-disable-next-line react-hooks/set-state-in-effect -- syncing derived state from machine selection
        setWorkingDir(machine.home_dir);
      }
    }
  }, [machineId, machines, workingDirDirty]);

  useEffect(() => {
    if (!open) {
      // eslint-disable-next-line react-hooks/set-state-in-effect -- resetting form on close
      setSessionType('claude');
      if (!preselectedMachineId) setMachineId('');
      setWorkingDir('');
      setWorkingDirDirty(false);
      setAdditionalArgs('');
      setModel('');
      setSkipPermissions('');
      setSelectedTemplate(null);
      setVariables({});
      setBrowseOpen(false);
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

      const args = additionalArgs.trim() ? additionalArgs.trim().split(/\s+/) : undefined;

      const session = await createSession.mutateAsync({
        machine_id: machineId,
        terminal_size: { cols, rows },
        ...(sessionType === 'terminal' ? { command: 'bash' } : {}),
        ...(workingDir ? { working_dir: workingDir } : {}),
        ...(sessionType === 'claude' && args ? { args } : {}),
        ...(sessionType === 'claude' && model ? { model } : {}),
        ...(sessionType === 'claude' && skipPermissions ? { skip_permissions: skipPermissions === '1' } : {}),
        ...(sessionType === 'claude' && selectedTemplate ? { template_id: selectedTemplate.template_id } : {}),
        ...(sessionType === 'claude' && variableNames.length > 0 ? { variables } : {}),
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
          {/* Session Type Selector */}
          <div>
            <label className="block text-sm text-text-secondary mb-1">Session Type</label>
            <div className="flex rounded-md border border-gray-600 overflow-hidden">
              <button
                type="button"
                onClick={() => {
                  setSessionType('claude');
                }}
                className={`flex items-center justify-center gap-2 flex-1 px-3 py-2 text-sm font-medium transition-colors ${
                  sessionType === 'claude'
                    ? 'bg-accent-primary text-white'
                    : 'bg-bg-tertiary text-text-secondary hover:text-text-primary'
                }`}
              >
                <Bot size={16} />
                Claude
              </button>
              <button
                type="button"
                onClick={() => {
                  setSessionType('terminal');
                  setAdditionalArgs('');
                  setModel('');
                  setSkipPermissions('');
                  setSelectedTemplate(null);
                  setVariables({});
                }}
                className={`flex items-center justify-center gap-2 flex-1 px-3 py-2 text-sm font-medium transition-colors ${
                  sessionType === 'terminal'
                    ? 'bg-accent-primary text-white'
                    : 'bg-bg-tertiary text-text-secondary hover:text-text-primary'
                }`}
              >
                <Terminal size={16} />
                Terminal
              </button>
            </div>
          </div>

          {sessionType === 'claude' && (
            <div>
              <label className="block text-sm text-text-secondary mb-1">Template</label>
              <TemplatePicker
                onSelect={(template) => {
                  setSelectedTemplate(template);
                  setVariables({});
                  if (template.working_dir) {
                    setWorkingDir(template.working_dir);
                    setWorkingDirDirty(true);
                  }
                  if (template.machine_id) {
                    setMachineId(template.machine_id);
                  }
                }}
              />
              {selectedTemplate && (
                <p className="text-xs text-accent-primary mt-1">
                  Using template: {selectedTemplate.name}
                </p>
              )}
            </div>
          )}

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

          <div>
            <label className="block text-sm text-text-secondary mb-1">
              Working Directory <span className="text-text-secondary/50">(optional)</span>
            </label>
            <div className="flex gap-2">
              <input
                type="text"
                value={workingDir}
                onChange={(e) => {
                  setWorkingDir(e.target.value);
                  setWorkingDirDirty(true);
                }}
                placeholder="/home/user/project"
                className="flex-1 rounded-md bg-bg-tertiary border border-gray-600 text-text-primary text-sm px-3 py-2 focus:outline-none focus:ring-1 focus:ring-accent-primary placeholder:text-text-secondary/30"
              />
              <button
                type="button"
                disabled={!machineId}
                onClick={() => setBrowseOpen(true)}
                className="px-3 py-2 rounded-md bg-bg-tertiary border border-gray-600 text-text-secondary hover:text-text-primary transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                title="Browse directories"
              >
                <FolderOpen size={16} />
              </button>
            </div>
          </div>

          {sessionType === 'claude' && (
            <>
              <div>
                <label className="block text-sm text-text-secondary mb-1">
                  Additional Arguments <span className="text-text-secondary/50">(optional)</span>
                </label>
                <input
                  type="text"
                  value={additionalArgs}
                  onChange={(e) => setAdditionalArgs(e.target.value)}
                  placeholder="--resume abc123 --verbose"
                  className="w-full rounded-md bg-bg-tertiary border border-gray-600 text-text-primary text-sm px-3 py-2 focus:outline-none focus:ring-1 focus:ring-accent-primary placeholder:text-text-secondary/30"
                />
                <p className="text-xs text-text-secondary/50 mt-1">Extra CLI flags passed to claude</p>
              </div>

              <div>
                <label className="block text-sm text-text-secondary mb-1">
                  Model <span className="text-text-secondary/50">(optional)</span>
                </label>
                <select
                  value={model}
                  onChange={(e) => setModel(e.target.value)}
                  className="w-full rounded-md bg-bg-tertiary border border-gray-600 text-text-primary text-sm px-3 py-2 focus:outline-none focus:ring-1 focus:ring-accent-primary"
                >
                  <option value="">Default</option>
                  <option value="opus">Opus</option>
                  <option value="sonnet">Sonnet</option>
                  <option value="haiku">Haiku</option>
                  <option value="opusplan">Opus Plan</option>
                </select>
              </div>

              <div>
                <label className="block text-sm text-text-secondary mb-1">
                  Skip Permissions <span className="text-text-secondary/50">(optional)</span>
                </label>
                <select
                  value={skipPermissions}
                  onChange={(e) => setSkipPermissions(e.target.value)}
                  className="w-full rounded-md bg-bg-tertiary border border-gray-600 text-text-primary text-sm px-3 py-2 focus:outline-none focus:ring-1 focus:ring-accent-primary"
                >
                  <option value="">Default (from settings)</option>
                  <option value="1">On</option>
                  <option value="0">Off</option>
                </select>
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
            </>
          )}

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

      {browseOpen && machineId && (
        <DirectoryBrowserModal
          open={browseOpen}
          onClose={() => setBrowseOpen(false)}
          onSelect={(path) => {
            setWorkingDir(path);
            setWorkingDirDirty(true);
            setBrowseOpen(false);
          }}
          machineId={machineId}
          initialPath={workingDir || undefined}
        />
      )}
    </div>,
    document.body,
  );
}
