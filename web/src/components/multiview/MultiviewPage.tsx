import { useState, useCallback, useEffect } from 'react';
import { useParams } from 'react-router';
import { MultiviewToolbar } from './MultiviewToolbar';
import { PanelLayout } from './PanelLayout';
import { TerminalPane } from './TerminalPane';
import { SessionPicker } from './SessionPicker';
import { EmptyMultiview } from './EmptyMultiview';
import { useMultiviewStore } from '../../stores/multiview';
import type { Pane } from '../../types/multiview';

export function MultiviewPage() {
  const { workspaceId } = useParams<{ workspaceId?: string }>();
  const {
    activeWorkspace,
    focusedPaneId,
    loadWorkspace,
    setFocusedPane,
    swapSession,
    addPane,
    removePane,
  } = useMultiviewStore();

  const [maximizedPaneId, setMaximizedPaneId] = useState<string | null>(null);
  const [pickerTarget, setPickerTarget] = useState<string | null>(null);

  // Load workspace from URL param (validate UUID format first)
  useEffect(() => {
    const uuidRe = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
    if (workspaceId && uuidRe.test(workspaceId)) {
      loadWorkspace(workspaceId);
    }
  }, [workspaceId, loadWorkspace]);

  // Keyboard shortcuts
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (!activeWorkspace) return;
      const panes = activeWorkspace.panes;

      // Ctrl+Shift+M — toggle maximize
      if (e.ctrlKey && e.shiftKey && e.key === 'M') {
        e.preventDefault();
        if (focusedPaneId) {
          setMaximizedPaneId((prev) => (prev === focusedPaneId ? null : focusedPaneId));
        }
        return;
      }

      // Ctrl+Shift+1-6 — jump to pane by number
      if (e.ctrlKey && e.shiftKey && e.code >= 'Digit1' && e.code <= 'Digit6') {
        e.preventDefault();
        const index = parseInt(e.code.replace('Digit', '')) - 1;
        if (index < panes.length) {
          setFocusedPane(panes[index].id);
        }
        return;
      }

      // Escape — unfocus
      if (e.key === 'Escape') {
        setFocusedPane(null);
        return;
      }

      // Ctrl+Shift+Arrow — directional focus
      if (e.ctrlKey && e.shiftKey && ['ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown'].includes(e.key)) {
        e.preventDefault();
        if (!focusedPaneId) return;
        const currentIndex = panes.findIndex((p) => p.id === focusedPaneId);
        if (currentIndex < 0) return;

        let nextIndex = currentIndex;
        if (e.key === 'ArrowRight' || e.key === 'ArrowDown') {
          nextIndex = Math.min(currentIndex + 1, panes.length - 1);
        } else {
          nextIndex = Math.max(currentIndex - 1, 0);
        }
        if (nextIndex !== currentIndex) {
          setFocusedPane(panes[nextIndex].id);
        }
      }
    };

    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, [activeWorkspace, focusedPaneId, setFocusedPane]);

  const handlePickerSelect = useCallback(
    (sessionId: string) => {
      if (pickerTarget === '__new__') {
        addPane(sessionId);
      } else if (pickerTarget) {
        swapSession(pickerTarget, sessionId);
      }
      setPickerTarget(null);
    },
    [pickerTarget, addPane, swapSession],
  );

  const useWebGL = (activeWorkspace?.panes.length ?? 0) <= 4;

  if (!activeWorkspace) {
    return (
      <div className="flex flex-col h-full">
        <EmptyMultiview />
      </div>
    );
  }

  const excludeSessionIds = activeWorkspace.panes.map((p) => p.sessionId).filter(Boolean);

  const renderPane = (pane: Pane) => {
    if (maximizedPaneId && pane.id !== maximizedPaneId) return null;

    return (
      <TerminalPane
        key={pane.id}
        pane={pane}
        isFocused={focusedPaneId === pane.id}
        isMaximized={maximizedPaneId === pane.id}
        useWebGL={useWebGL}
        onFocus={() => setFocusedPane(pane.id)}
        onMaximize={() =>
          setMaximizedPaneId((prev) => (prev === pane.id ? null : pane.id))
        }
        onPickSession={() => setPickerTarget(pane.id)}
        onRemovePane={() => removePane(pane.id)}
        canRemove={activeWorkspace.panes.length > 2}
      />
    );
  };

  // When maximized, show only that pane
  if (maximizedPaneId) {
    const pane = activeWorkspace.panes.find((p) => p.id === maximizedPaneId);
    if (pane) {
      return (
        <div className="flex flex-col h-full">
          <MultiviewToolbar />
          <div className="flex-1 min-h-0 p-1">{renderPane(pane)}</div>
          {pickerTarget && (
            <SessionPicker
              onSelect={handlePickerSelect}
              onClose={() => setPickerTarget(null)}
              excludeSessionIds={excludeSessionIds}
            />
          )}
        </div>
      );
    }
  }

  return (
    <div className="flex flex-col h-full">
      <MultiviewToolbar />
      <div className="flex-1 min-h-0 p-1">
        <PanelLayout
          preset={activeWorkspace.layout.preset}
          panes={[...activeWorkspace.panes]}
          renderPane={renderPane}
          workspaceId={activeWorkspace.id}
        />
      </div>
      {pickerTarget && (
        <SessionPicker
          onSelect={handlePickerSelect}
          onClose={() => setPickerTarget(null)}
          excludeSessionIds={excludeSessionIds}
        />
      )}
    </div>
  );
}
