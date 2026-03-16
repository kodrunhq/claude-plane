import { Fragment, type ReactNode } from 'react';
import { PanelGroup, Panel, PanelResizeHandle } from 'react-resizable-panels';
import type { LayoutPreset, Pane } from '../../types/multiview';

interface PanelLayoutProps {
  readonly preset: LayoutPreset;
  readonly panes: readonly Pane[];
  readonly renderPane: (pane: Pane) => ReactNode;
  readonly workspaceId: string;
}

function ResizeHandle() {
  return (
    <PanelResizeHandle className="w-1 hover:w-1.5 bg-border-primary hover:bg-accent-primary transition-all duration-150" />
  );
}

function VerticalResizeHandle() {
  return (
    <PanelResizeHandle className="h-1 hover:h-1.5 bg-border-primary hover:bg-accent-primary transition-all duration-150" />
  );
}

function HorizontalRow({
  panes,
  renderPane,
  autoSaveId,
  minSize,
}: {
  readonly panes: readonly Pane[];
  readonly renderPane: (pane: Pane) => ReactNode;
  readonly autoSaveId: string;
  readonly minSize?: number;
}) {
  return (
    <PanelGroup direction="horizontal" autoSaveId={autoSaveId}>
      {panes.map((pane, i) => (
        <Fragment key={pane.id}>
          {i > 0 && <ResizeHandle />}
          <Panel minSize={minSize ?? 15}>
            {renderPane(pane)}
          </Panel>
        </Fragment>
      ))}
    </PanelGroup>
  );
}

export function PanelLayout({ preset, panes, renderPane, workspaceId }: PanelLayoutProps) {
  const baseId = `multiview-${workspaceId}`;

  switch (preset) {
    case '2-horizontal':
    case 'custom':
      return (
        <PanelGroup direction="horizontal" autoSaveId={`${baseId}-h`}>
          <Panel minSize={15}>{renderPane(panes[0])}</Panel>
          <ResizeHandle />
          <Panel minSize={15}>{renderPane(panes[1])}</Panel>
        </PanelGroup>
      );

    case '2-vertical':
      return (
        <PanelGroup direction="vertical" autoSaveId={`${baseId}-v`}>
          <Panel minSize={15}>{renderPane(panes[0])}</Panel>
          <VerticalResizeHandle />
          <Panel minSize={15}>{renderPane(panes[1])}</Panel>
        </PanelGroup>
      );

    case '3-columns':
      return (
        <HorizontalRow panes={panes.slice(0, 3)} renderPane={renderPane} autoSaveId={`${baseId}-h`} minSize={10} />
      );

    case '3-main-side':
      return (
        <PanelGroup direction="horizontal" autoSaveId={`${baseId}-h`}>
          <Panel defaultSize={66} minSize={30}>
            {renderPane(panes[0])}
          </Panel>
          <ResizeHandle />
          <Panel minSize={15}>
            <PanelGroup direction="vertical" autoSaveId={`${baseId}-v-right`}>
              <Panel minSize={20}>{renderPane(panes[1])}</Panel>
              <VerticalResizeHandle />
              <Panel minSize={20}>{renderPane(panes[2])}</Panel>
            </PanelGroup>
          </Panel>
        </PanelGroup>
      );

    case '4-grid':
      return (
        <PanelGroup direction="vertical" autoSaveId={`${baseId}-v`}>
          <Panel minSize={20}>
            <HorizontalRow panes={panes.slice(0, 2)} renderPane={renderPane} autoSaveId={`${baseId}-h-top`} />
          </Panel>
          <VerticalResizeHandle />
          <Panel minSize={20}>
            <HorizontalRow panes={panes.slice(2, 4)} renderPane={renderPane} autoSaveId={`${baseId}-h-bot`} />
          </Panel>
        </PanelGroup>
      );

    case '5-grid':
      return (
        <PanelGroup direction="vertical" autoSaveId={`${baseId}-v`}>
          <Panel minSize={20}>
            <HorizontalRow panes={panes.slice(0, 3)} renderPane={renderPane} autoSaveId={`${baseId}-h-top`} minSize={10} />
          </Panel>
          <VerticalResizeHandle />
          <Panel minSize={20}>
            <HorizontalRow panes={panes.slice(3, 5)} renderPane={renderPane} autoSaveId={`${baseId}-h-bot`} />
          </Panel>
        </PanelGroup>
      );

    case '6-grid':
      return (
        <PanelGroup direction="vertical" autoSaveId={`${baseId}-v`}>
          <Panel minSize={20}>
            <HorizontalRow panes={panes.slice(0, 3)} renderPane={renderPane} autoSaveId={`${baseId}-h-top`} minSize={10} />
          </Panel>
          <VerticalResizeHandle />
          <Panel minSize={20}>
            <HorizontalRow panes={panes.slice(3, 6)} renderPane={renderPane} autoSaveId={`${baseId}-h-bot`} minSize={10} />
          </Panel>
        </PanelGroup>
      );

    default:
      return (
        <PanelGroup direction="horizontal" autoSaveId={`${baseId}-h`}>
          <Panel minSize={15}>{renderPane(panes[0])}</Panel>
          <ResizeHandle />
          <Panel minSize={15}>{renderPane(panes[1])}</Panel>
        </PanelGroup>
      );
  }
}
