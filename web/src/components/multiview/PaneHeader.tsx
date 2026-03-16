import { useState } from 'react';
import { Maximize2, Minimize2, Sparkles, TerminalSquare } from 'lucide-react';

interface PaneHeaderProps {
  readonly sessionType: 'claude' | 'terminal';
  readonly machineName: string;
  readonly workingDir: string;
  readonly isMaximized: boolean;
  readonly onMaximize: () => void;
  readonly onSwapSession?: () => void;
  readonly onRemovePane?: () => void;
  readonly onOpenFullView?: () => void;
  readonly canRemove?: boolean;
}

function truncateDir(dir: string, maxLen: number = 30): string {
  if (dir.length <= maxLen) return dir;
  return '\u2026' + dir.slice(-(maxLen - 1));
}

export function PaneHeader({
  sessionType,
  machineName,
  workingDir,
  isMaximized,
  onMaximize,
  onSwapSession,
  onRemovePane,
  onOpenFullView,
  canRemove,
}: PaneHeaderProps) {
  const [contextMenu, setContextMenu] = useState<{ x: number; y: number } | null>(null);
  const Icon = sessionType === 'claude' ? Sparkles : TerminalSquare;

  const handleContextMenu = (e: React.MouseEvent) => {
    e.preventDefault();
    const menuWidth = 160;
    const menuHeight = 120;
    const x = Math.min(e.clientX, window.innerWidth - menuWidth);
    const y = Math.min(e.clientY, window.innerHeight - menuHeight);
    setContextMenu({ x, y });
  };

  return (
    <div
      className="flex items-center h-6 px-2 bg-bg-secondary border-b border-border-primary text-xs text-text-secondary select-none shrink-0"
      onContextMenu={handleContextMenu}
    >
      <Icon size={12} className="shrink-0 mr-1.5" />
      <span className="font-medium text-text-primary mr-2 shrink-0">{machineName}</span>
      <span className="truncate" title={workingDir}>
        {truncateDir(workingDir)}
      </span>
      <div className="ml-auto shrink-0">
        <button
          onClick={(e) => {
            e.stopPropagation();
            onMaximize();
          }}
          className="p-0.5 rounded hover:bg-bg-tertiary transition-colors"
          aria-label={isMaximized ? 'Restore pane' : 'Maximize pane'}
        >
          {isMaximized ? <Minimize2 size={12} /> : <Maximize2 size={12} />}
        </button>
      </div>

      {contextMenu && (
        <>
          <div className="fixed inset-0 z-50" onClick={() => setContextMenu(null)} />
          <div
            className="fixed z-50 bg-bg-secondary border border-border-primary rounded-lg shadow-xl py-1 min-w-40"
            style={{ left: contextMenu.x, top: contextMenu.y }}
          >
            {onSwapSession && (
              <button
                onClick={() => { onSwapSession(); setContextMenu(null); }}
                className="w-full text-left px-3 py-1.5 text-sm text-text-primary hover:bg-bg-tertiary"
              >
                Swap session...
              </button>
            )}
            {onRemovePane && canRemove && (
              <button
                onClick={() => { onRemovePane(); setContextMenu(null); }}
                className="w-full text-left px-3 py-1.5 text-sm text-text-primary hover:bg-bg-tertiary"
              >
                Remove pane
              </button>
            )}
            {onOpenFullView && (
              <button
                onClick={() => { onOpenFullView(); setContextMenu(null); }}
                className="w-full text-left px-3 py-1.5 text-sm text-text-primary hover:bg-bg-tertiary"
              >
                Open in full view
              </button>
            )}
          </div>
        </>
      )}
    </div>
  );
}
