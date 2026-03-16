import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { PaneHeader } from '../../../components/multiview/PaneHeader';

const defaultProps = {
  sessionType: 'claude' as const,
  machineName: 'worker-1',
  workingDir: '/home/user/projects/my-app',
  isMaximized: false,
  onMaximize: vi.fn(),
};

describe('PaneHeader', () => {
  it('renders machine name and working directory', () => {
    render(<PaneHeader {...defaultProps} />);
    expect(screen.getByText('worker-1')).toBeDefined();
    expect(screen.getByText(/my-app/)).toBeDefined();
  });

  it('truncates long working directory from the left with ellipsis', () => {
    render(
      <PaneHeader {...defaultProps} workingDir="/very/long/path/that/should/be/truncated/to/show/end" />,
    );
    const span = screen.getByTitle('/very/long/path/that/should/be/truncated/to/show/end');
    expect(span.textContent).toMatch(/^\u2026/);
    expect(span.textContent).toMatch(/show\/end$/);
  });

  it('does not truncate directory at exactly 30 chars', () => {
    const dir = '/a/path/exactly/thirty/chars!';
    render(<PaneHeader {...defaultProps} workingDir={dir.padEnd(30, '/')} />);
    const span = screen.getByTitle(dir.padEnd(30, '/'));
    expect(span.textContent).not.toMatch(/^\u2026/);
  });

  it('shows Maximize2 icon when not maximized', () => {
    render(<PaneHeader {...defaultProps} />);
    expect(screen.getByLabelText('Maximize pane')).toBeDefined();
  });

  it('shows Minimize2 icon when maximized', () => {
    render(<PaneHeader {...defaultProps} isMaximized={true} />);
    expect(screen.getByLabelText('Restore pane')).toBeDefined();
  });

  it('calls onMaximize when button is clicked', () => {
    const onMaximize = vi.fn();
    render(<PaneHeader {...defaultProps} onMaximize={onMaximize} />);
    fireEvent.click(screen.getByLabelText('Maximize pane'));
    expect(onMaximize).toHaveBeenCalledOnce();
  });

  it('opens context menu on right-click of header', () => {
    const onSwapSession = vi.fn();
    render(<PaneHeader {...defaultProps} onSwapSession={onSwapSession} />);

    const header = screen.getByText('worker-1').closest('div')!;
    fireEvent.contextMenu(header);

    expect(screen.getByText('Swap session...')).toBeDefined();
  });

  it('shows Remove pane only when onRemovePane and canRemove are both true', () => {
    const onRemovePane = vi.fn();

    // canRemove false — should not show
    const { unmount } = render(
      <PaneHeader {...defaultProps} onRemovePane={onRemovePane} canRemove={false} />,
    );
    const header1 = screen.getByText('worker-1').closest('div')!;
    fireEvent.contextMenu(header1);
    expect(screen.queryByText('Remove pane')).toBeNull();
    unmount();

    // canRemove true — should show
    render(<PaneHeader {...defaultProps} onRemovePane={onRemovePane} canRemove={true} />);
    const header2 = screen.getByText('worker-1').closest('div')!;
    fireEvent.contextMenu(header2);
    expect(screen.getByText('Remove pane')).toBeDefined();
  });

  it('renders Sparkles icon for claude session type', () => {
    const { container } = render(<PaneHeader {...defaultProps} sessionType="claude" />);
    // Sparkles is an SVG rendered by lucide-react
    expect(container.querySelector('svg')).toBeDefined();
  });

  it('closes context menu when clicking backdrop', () => {
    render(<PaneHeader {...defaultProps} onSwapSession={vi.fn()} />);

    const header = screen.getByText('worker-1').closest('div')!;
    fireEvent.contextMenu(header);
    expect(screen.getByText('Swap session...')).toBeDefined();

    // Click the backdrop (fixed inset-0 div)
    const backdrop = document.querySelector('.fixed.inset-0.z-50') as HTMLElement;
    fireEvent.click(backdrop);
    expect(screen.queryByText('Swap session...')).toBeNull();
  });

  it('calls onSwapSession when clicking Swap session menu item', () => {
    const onSwapSession = vi.fn();
    render(<PaneHeader {...defaultProps} onSwapSession={onSwapSession} />);

    const header = screen.getByText('worker-1').closest('div')!;
    fireEvent.contextMenu(header);
    fireEvent.click(screen.getByText('Swap session...'));

    expect(onSwapSession).toHaveBeenCalledOnce();
  });
});
