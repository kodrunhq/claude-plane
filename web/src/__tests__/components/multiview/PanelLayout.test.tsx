import { describe, it, expect, vi } from 'vitest';
import { render } from '@testing-library/react';
import { PanelLayout } from '../../../components/multiview/PanelLayout';
import type { Pane } from '../../../types/multiview';

// Mock react-resizable-panels to avoid DOM measurement issues in jsdom
vi.mock('react-resizable-panels', () => ({
  Group: ({ children, ...props }: any) => (
    <div data-testid="panel-group" data-orientation={props.orientation}>{children}</div>
  ),
  Panel: ({ children }: any) => (
    <div data-testid="panel">{children}</div>
  ),
  Separator: () => <div data-testid="separator" />,
}));

const makePanes = (count: number): Pane[] =>
  Array.from({ length: count }, (_, i) => ({
    id: `pane-${i}`,
    sessionId: `session-${i}`,
  }));

describe('PanelLayout', () => {
  it('renders 2-horizontal with correct pane order', () => {
    const renderPane = vi.fn((pane: Pane) => <div data-testid={`content-${pane.id}`} />);
    render(
      <PanelLayout preset="2-horizontal" panes={makePanes(2)} renderPane={renderPane} workspaceId="ws-1" />,
    );
    expect(renderPane).toHaveBeenCalledTimes(2);
    expect(renderPane.mock.calls[0][0].id).toBe('pane-0');
    expect(renderPane.mock.calls[1][0].id).toBe('pane-1');
  });

  it('renders 4-grid with 4 panes in correct reading order', () => {
    const renderPane = vi.fn((pane: Pane) => <div data-testid={`content-${pane.id}`} />);
    render(
      <PanelLayout preset="4-grid" panes={makePanes(4)} renderPane={renderPane} workspaceId="ws-1" />,
    );
    expect(renderPane).toHaveBeenCalledTimes(4);
    expect(renderPane.mock.calls.map((c: any[]) => c[0].id)).toEqual([
      'pane-0', 'pane-1', 'pane-2', 'pane-3',
    ]);
  });

  it('renders 5-grid with 3 top + 2 bottom', () => {
    const renderPane = vi.fn((_pane: Pane) => <div />);
    render(
      <PanelLayout preset="5-grid" panes={makePanes(5)} renderPane={renderPane} workspaceId="ws-1" />,
    );
    expect(renderPane).toHaveBeenCalledTimes(5);
    // First 3 are top row, last 2 are bottom
    expect(renderPane.mock.calls[0][0].id).toBe('pane-0');
    expect(renderPane.mock.calls[2][0].id).toBe('pane-2');
    expect(renderPane.mock.calls[3][0].id).toBe('pane-3');
    expect(renderPane.mock.calls[4][0].id).toBe('pane-4');
  });

  it('renders fallback message when pane count is less than preset requires', () => {
    const renderPane = vi.fn();
    const { container } = render(
      <PanelLayout preset="4-grid" panes={makePanes(2)} renderPane={renderPane} workspaceId="ws-1" />,
    );
    expect(renderPane).not.toHaveBeenCalled();
    expect(container.textContent).toContain('Layout requires at least 4 panes');
  });

  it('renders 3-main-side with pane[0] as main and panes[1,2] as side', () => {
    const renderPane = vi.fn((_pane: Pane) => <div />);
    render(
      <PanelLayout preset="3-main-side" panes={makePanes(3)} renderPane={renderPane} workspaceId="ws-1" />,
    );
    expect(renderPane).toHaveBeenCalledTimes(3);
    expect(renderPane.mock.calls[0][0].id).toBe('pane-0');
    expect(renderPane.mock.calls[1][0].id).toBe('pane-1');
    expect(renderPane.mock.calls[2][0].id).toBe('pane-2');
  });
});
