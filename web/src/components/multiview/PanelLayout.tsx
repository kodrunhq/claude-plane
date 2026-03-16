import { Fragment, type ReactNode } from 'react';
import { Group, Panel, Separator } from 'react-resizable-panels';
import type { LayoutPreset, Pane } from '../../types/multiview';

interface PanelLayoutProps {
  readonly preset: LayoutPreset;
  readonly panes: readonly Pane[];
  readonly renderPane: (pane: Pane) => ReactNode;
  readonly workspaceId: string;
}

function ResizeHandle() {
  return (
    <Separator className="w-1 hover:w-1.5 bg-border-primary hover:bg-accent-primary transition-all duration-150" />
  );
}

function VerticalResizeHandle() {
  return (
    <Separator className="h-1 hover:h-1.5 bg-border-primary hover:bg-accent-primary transition-all duration-150" />
  );
}

function HorizontalRow({
  panes,
  renderPane,
  groupId,
  minSize,
}: {
  readonly panes: readonly Pane[];
  readonly renderPane: (pane: Pane) => ReactNode;
  readonly groupId: string;
  readonly minSize?: number;
}) {
  return (
    <Group orientation="horizontal" id={groupId}>
      {panes.map((pane, i) => (
        <Fragment key={pane.id}>
          {i > 0 && <ResizeHandle />}
          <Panel minSize={minSize ?? 15}>
            {renderPane(pane)}
          </Panel>
        </Fragment>
      ))}
    </Group>
  );
}

export function PanelLayout({ preset, panes, renderPane, workspaceId }: PanelLayoutProps) {
  const baseId = `multiview-${workspaceId}`;

  switch (preset) {
    case '2-horizontal':
    case 'custom':
      return (
        <Group orientation="horizontal" id={`${baseId}-h`}>
          <Panel minSize={15}>{renderPane(panes[0])}</Panel>
          <ResizeHandle />
          <Panel minSize={15}>{renderPane(panes[1])}</Panel>
        </Group>
      );

    case '2-vertical':
      return (
        <Group orientation="vertical" id={`${baseId}-v`}>
          <Panel minSize={15}>{renderPane(panes[0])}</Panel>
          <VerticalResizeHandle />
          <Panel minSize={15}>{renderPane(panes[1])}</Panel>
        </Group>
      );

    case '3-columns':
      return (
        <HorizontalRow panes={panes.slice(0, 3)} renderPane={renderPane} groupId={`${baseId}-h`} minSize={10} />
      );

    case '3-main-side':
      return (
        <Group orientation="horizontal" id={`${baseId}-h`}>
          <Panel defaultSize={66} minSize={30}>
            {renderPane(panes[0])}
          </Panel>
          <ResizeHandle />
          <Panel minSize={15}>
            <Group orientation="vertical" id={`${baseId}-v-right`}>
              <Panel minSize={20}>{renderPane(panes[1])}</Panel>
              <VerticalResizeHandle />
              <Panel minSize={20}>{renderPane(panes[2])}</Panel>
            </Group>
          </Panel>
        </Group>
      );

    case '4-grid':
      return (
        <Group orientation="vertical" id={`${baseId}-v`}>
          <Panel minSize={20}>
            <HorizontalRow panes={panes.slice(0, 2)} renderPane={renderPane} groupId={`${baseId}-h-top`} />
          </Panel>
          <VerticalResizeHandle />
          <Panel minSize={20}>
            <HorizontalRow panes={panes.slice(2, 4)} renderPane={renderPane} groupId={`${baseId}-h-bot`} />
          </Panel>
        </Group>
      );

    case '5-grid':
      return (
        <Group orientation="vertical" id={`${baseId}-v`}>
          <Panel minSize={20}>
            <HorizontalRow panes={panes.slice(0, 3)} renderPane={renderPane} groupId={`${baseId}-h-top`} minSize={10} />
          </Panel>
          <VerticalResizeHandle />
          <Panel minSize={20}>
            <HorizontalRow panes={panes.slice(3, 5)} renderPane={renderPane} groupId={`${baseId}-h-bot`} />
          </Panel>
        </Group>
      );

    case '6-grid':
      return (
        <Group orientation="vertical" id={`${baseId}-v`}>
          <Panel minSize={20}>
            <HorizontalRow panes={panes.slice(0, 3)} renderPane={renderPane} groupId={`${baseId}-h-top`} minSize={10} />
          </Panel>
          <VerticalResizeHandle />
          <Panel minSize={20}>
            <HorizontalRow panes={panes.slice(3, 6)} renderPane={renderPane} groupId={`${baseId}-h-bot`} minSize={10} />
          </Panel>
        </Group>
      );

    default:
      return (
        <Group orientation="horizontal" id={`${baseId}-h`}>
          <Panel minSize={15}>{renderPane(panes[0])}</Panel>
          <ResizeHandle />
          <Panel minSize={15}>{renderPane(panes[1])}</Panel>
        </Group>
      );
  }
}
