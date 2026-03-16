import { useState, useRef, useEffect } from 'react';
import { Save, Copy, ChevronDown, Plus, Trash2 } from 'lucide-react';
import { LayoutPresetIcon } from './LayoutPresetIcon';
import { ConfirmDialog } from '../shared/ConfirmDialog';
import { useMultiviewStore } from '../../stores/multiview';
import type { LayoutPreset, Workspace } from '../../types/multiview';

const PRESETS: readonly LayoutPreset[] = [
  '2-horizontal', '2-vertical',
  '3-columns', '3-main-side',
  '4-grid', '5-grid', '6-grid',
];

const PRESET_LABELS: Record<LayoutPreset, string> = {
  '2-horizontal': '2 Side by Side',
  '2-vertical': '2 Stacked',
  '3-columns': '3 Columns',
  '3-main-side': '1 Main + 2 Side',
  '4-grid': '2\u00d72 Grid',
  '5-grid': '3+2 Grid',
  '6-grid': '2\u00d73 Grid',
  custom: 'Custom',
};

const MIN_PANES: Record<LayoutPreset, number> = {
  '2-horizontal': 2, '2-vertical': 2,
  '3-columns': 3, '3-main-side': 3,
  '4-grid': 4, '5-grid': 5, '6-grid': 6,
  custom: 2,
};

export function MultiviewToolbar() {
  const {
    activeWorkspace,
    workspaces,
    setLayoutPreset,
    saveWorkspace,
    loadWorkspace,
    deleteWorkspace,
    renameWorkspace,
    addPane,
  } = useMultiviewStore();

  const [isEditing, setIsEditing] = useState(false);
  const [editName, setEditName] = useState('');
  const [showSwitcher, setShowSwitcher] = useState(false);
  const [showSavePrompt, setShowSavePrompt] = useState(false);
  const [saveName, setSaveName] = useState('');
  const [workspaceToDelete, setWorkspaceToDelete] = useState<Workspace | null>(null);
  const nameInputRef = useRef<HTMLInputElement>(null);
  const switcherRef = useRef<HTMLDivElement>(null);

  const currentPreset = activeWorkspace?.layout.preset ?? '2-horizontal';
  const paneCount = activeWorkspace?.panes.length ?? 0;

  const availablePresets = PRESETS.filter((p) => MIN_PANES[p] <= paneCount);

  useEffect(() => {
    if (isEditing && nameInputRef.current) {
      nameInputRef.current.focus();
      nameInputRef.current.select();
    }
  }, [isEditing]);

  useEffect(() => {
    const handler = (e: MouseEvent) => {
      if (switcherRef.current && !switcherRef.current.contains(e.target as Node)) {
        setShowSwitcher(false);
      }
    };
    document.addEventListener('mousedown', handler);
    return () => document.removeEventListener('mousedown', handler);
  }, []);

  const handleSave = () => {
    if (activeWorkspace?.name) {
      saveWorkspace(activeWorkspace.name);
    } else {
      setShowSavePrompt(true);
      setSaveName('');
    }
  };

  const handleSaveConfirm = () => {
    if (saveName.trim()) {
      saveWorkspace(saveName.trim());
      setShowSavePrompt(false);
    }
  };

  const handleRename = () => {
    if (activeWorkspace && editName.trim()) {
      renameWorkspace(activeWorkspace.id, editName.trim());
    }
    setIsEditing(false);
  };

  return (
    <div className="flex items-center gap-3 px-4 py-2 border-b border-border-primary bg-bg-secondary shrink-0">
      {/* Layout Presets */}
      <div className="flex items-center gap-1">
        {availablePresets.map((preset) => (
          <button
            key={preset}
            onClick={() => setLayoutPreset(preset)}
            className={`p-1 rounded transition-colors ${
              currentPreset === preset ? 'bg-bg-tertiary' : 'hover:bg-bg-tertiary'
            }`}
            title={PRESET_LABELS[preset]}
          >
            <LayoutPresetIcon preset={preset} size={20} active={currentPreset === preset} />
          </button>
        ))}
      </div>

      <div className="w-px h-5 bg-border-primary" />

      {/* Workspace Name */}
      <div className="flex items-center gap-2">
        {isEditing ? (
          <input
            ref={nameInputRef}
            value={editName}
            onChange={(e) => setEditName(e.target.value)}
            onBlur={handleRename}
            onKeyDown={(e) => {
              if (e.key === 'Enter') handleRename();
              if (e.key === 'Escape') setIsEditing(false);
            }}
            maxLength={100}
            className="px-2 py-0.5 text-sm bg-bg-primary border border-border-primary rounded text-text-primary focus:outline-none focus:ring-1 focus:ring-accent-primary"
          />
        ) : (
          <button
            onClick={() => {
              setEditName(activeWorkspace?.name ?? '');
              setIsEditing(true);
            }}
            className="text-sm text-text-primary hover:text-accent-primary transition-colors"
          >
            {activeWorkspace?.name ?? 'Untitled workspace'}
          </button>
        )}

        {/* Save */}
        <button
          onClick={handleSave}
          className="p-1 rounded hover:bg-bg-tertiary text-text-secondary hover:text-text-primary transition-colors"
          title="Save workspace"
        >
          <Save size={14} />
        </button>

        {/* Save As */}
        <button
          onClick={() => { setShowSavePrompt(true); setSaveName(''); }}
          className="p-1 rounded hover:bg-bg-tertiary text-text-secondary hover:text-text-primary transition-colors"
          title="Save as new workspace"
        >
          <Copy size={14} />
        </button>

        {/* Workspace Switcher */}
        <div className="relative" ref={switcherRef}>
          <button
            onClick={() => setShowSwitcher(!showSwitcher)}
            className="p-1 rounded hover:bg-bg-tertiary text-text-secondary hover:text-text-primary transition-colors"
            title="Switch workspace"
          >
            <ChevronDown size={14} />
          </button>
          {showSwitcher && (
            <div className="absolute top-full left-0 mt-1 w-56 bg-bg-secondary border border-border-primary rounded-lg shadow-xl z-50">
              {workspaces.map((ws) => (
                <div key={ws.id} className="flex items-center px-3 py-2 hover:bg-bg-tertiary group">
                  <button
                    onClick={() => {
                      loadWorkspace(ws.id);
                      setShowSwitcher(false);
                    }}
                    className="flex-1 text-left text-sm text-text-primary truncate"
                  >
                    {ws.name ?? 'Untitled'}
                  </button>
                  <button
                    onClick={(e) => {
                      e.stopPropagation();
                      setWorkspaceToDelete(ws);
                    }}
                    className="p-1 rounded opacity-0 group-hover:opacity-100 hover:bg-bg-primary text-text-secondary"
                  >
                    <Trash2 size={12} />
                  </button>
                </div>
              ))}
              {workspaces.length === 0 && (
                <p className="px-3 py-2 text-xs text-text-secondary">No saved workspaces</p>
              )}
            </div>
          )}
        </div>
      </div>

      <div className="flex-1" />

      {/* Add Pane */}
      {paneCount < 6 && (
        <button
          onClick={() => addPane('')}
          className="flex items-center gap-1 px-2 py-1 text-xs rounded bg-bg-tertiary text-text-secondary hover:text-text-primary hover:bg-border-primary transition-colors"
        >
          <Plus size={12} />
          Add pane
        </button>
      )}

      {/* Delete Confirmation */}
      <ConfirmDialog
        open={workspaceToDelete !== null}
        title="Delete Workspace"
        message={`Delete workspace "${workspaceToDelete?.name ?? 'Untitled'}"? This cannot be undone.`}
        confirmLabel="Delete"
        variant="danger"
        onConfirm={() => {
          if (workspaceToDelete) deleteWorkspace(workspaceToDelete.id);
          setWorkspaceToDelete(null);
        }}
        onCancel={() => setWorkspaceToDelete(null)}
      />

      {/* Save Prompt Modal */}
      {showSavePrompt && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50" onClick={() => setShowSavePrompt(false)}>
          <div className="bg-bg-secondary p-4 rounded-lg shadow-xl w-80" onClick={(e) => e.stopPropagation()}>
            <h3 className="text-sm font-semibold text-text-primary mb-3">Save Workspace</h3>
            <input
              value={saveName}
              onChange={(e) => setSaveName(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && handleSaveConfirm()}
              placeholder="Workspace name"
              maxLength={100}
              className="w-full px-3 py-2 text-sm bg-bg-primary border border-border-primary rounded text-text-primary mb-3 focus:outline-none focus:ring-1 focus:ring-accent-primary"
              autoFocus
            />
            <div className="flex justify-end gap-2">
              <button
                onClick={() => setShowSavePrompt(false)}
                className="px-3 py-1.5 text-xs rounded text-text-secondary hover:text-text-primary"
              >
                Cancel
              </button>
              <button
                onClick={handleSaveConfirm}
                disabled={!saveName.trim()}
                className="px-3 py-1.5 text-xs rounded bg-accent-primary text-white hover:bg-accent-primary/80 disabled:opacity-50"
              >
                Save
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
