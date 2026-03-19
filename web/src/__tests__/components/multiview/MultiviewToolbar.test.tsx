import { describe, it, expect, vi, beforeEach } from 'vitest';
import { screen, fireEvent } from '@testing-library/react';
import { MultiviewToolbar } from '../../../components/multiview/MultiviewToolbar';
import { renderWithProviders } from '../../../test/render';
import { useMultiviewStore } from '../../../stores/multiview';
import type { Workspace } from '../../../types/multiview';

function makeWorkspace(overrides?: Partial<Workspace>): Workspace {
  return {
    id: 'ws-1',
    name: 'My Workspace',
    layout: { preset: '2-horizontal' },
    panes: [
      { id: 'p-1', sessionId: 's-1' },
      { id: 'p-2', sessionId: 's-2' },
      { id: 'p-3', sessionId: 's-3' },
      { id: 'p-4', sessionId: 's-4' },
    ],
    createdAt: '2026-01-01T00:00:00Z',
    updatedAt: '2026-01-01T00:00:00Z',
    ...overrides,
  };
}

describe('MultiviewToolbar', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    useMultiviewStore.setState({
      activeWorkspace: makeWorkspace(),
      workspaces: [],
      focusedPaneId: null,
    });
  });

  it('renders layout preset buttons based on pane count', () => {
    renderWithProviders(<MultiviewToolbar />);
    // With 4 panes: 2-horizontal, 2-vertical, 3-columns, 3-main-side, 4-grid should be available
    expect(screen.getByTitle('2 Side by Side')).toBeInTheDocument();
    expect(screen.getByTitle('2 Stacked')).toBeInTheDocument();
    expect(screen.getByTitle('3 Columns')).toBeInTheDocument();
    expect(screen.getByTitle('1 Main + 2 Side')).toBeInTheDocument();
    expect(screen.getByTitle('2\u00d72 Grid')).toBeInTheDocument();
    // 5-grid and 6-grid require 5+ and 6+ panes respectively
    expect(screen.queryByTitle('3+2 Grid')).not.toBeInTheDocument();
    expect(screen.queryByTitle('2\u00d73 Grid')).not.toBeInTheDocument();
  });

  it('calls setLayoutPreset when a preset button is clicked', async () => {
    const setLayoutPreset = vi.fn();
    useMultiviewStore.setState({ setLayoutPreset } as never);
    const { user } = renderWithProviders(<MultiviewToolbar />);
    await user.click(screen.getByTitle('2 Stacked'));
    expect(setLayoutPreset).toHaveBeenCalledWith('2-vertical');
  });

  it('renders workspace name', () => {
    renderWithProviders(<MultiviewToolbar />);
    expect(screen.getByText('My Workspace')).toBeInTheDocument();
  });

  it('shows "Untitled workspace" when name is null', () => {
    useMultiviewStore.setState({
      activeWorkspace: makeWorkspace({ name: null }),
    });
    renderWithProviders(<MultiviewToolbar />);
    expect(screen.getByText('Untitled workspace')).toBeInTheDocument();
  });

  it('renders Add pane button when pane count < 6', () => {
    renderWithProviders(<MultiviewToolbar />);
    expect(screen.getByText('Add pane')).toBeInTheDocument();
  });

  it('does not render Add pane button when pane count is 6', () => {
    useMultiviewStore.setState({
      activeWorkspace: makeWorkspace({
        panes: [
          { id: 'p1', sessionId: 's1' },
          { id: 'p2', sessionId: 's2' },
          { id: 'p3', sessionId: 's3' },
          { id: 'p4', sessionId: 's4' },
          { id: 'p5', sessionId: 's5' },
          { id: 'p6', sessionId: 's6' },
        ],
      }),
    });
    renderWithProviders(<MultiviewToolbar />);
    expect(screen.queryByText('Add pane')).not.toBeInTheDocument();
  });

  it('fires onAddPane when Add pane button is clicked', async () => {
    const onAddPane = vi.fn();
    const { user } = renderWithProviders(
      <MultiviewToolbar onAddPane={onAddPane} />,
    );
    await user.click(screen.getByText('Add pane'));
    expect(onAddPane).toHaveBeenCalledOnce();
  });

  it('calls saveWorkspace with existing name when save button is clicked', async () => {
    const saveWorkspace = vi.fn();
    useMultiviewStore.setState({ saveWorkspace } as never);
    const { user } = renderWithProviders(<MultiviewToolbar />);
    await user.click(screen.getByTitle('Save workspace'));
    expect(saveWorkspace).toHaveBeenCalledWith('My Workspace');
  });

  it('opens save prompt when save is clicked on unnamed workspace', async () => {
    useMultiviewStore.setState({
      activeWorkspace: makeWorkspace({ name: null }),
    });
    const { user } = renderWithProviders(<MultiviewToolbar />);
    await user.click(screen.getByTitle('Save workspace'));
    expect(screen.getByText('Save Workspace')).toBeInTheDocument();
    expect(screen.getByPlaceholderText('Workspace name')).toBeInTheDocument();
  });

  it('opens save-as modal when Save As button is clicked', async () => {
    const { user } = renderWithProviders(<MultiviewToolbar />);
    await user.click(screen.getByTitle('Save as new workspace'));
    expect(screen.getByText('Save As New Workspace')).toBeInTheDocument();
  });

  it('opens workspace switcher dropdown when chevron is clicked', async () => {
    useMultiviewStore.setState({
      workspaces: [makeWorkspace({ id: 'ws-saved', name: 'Saved WS' })],
    });
    const { user } = renderWithProviders(<MultiviewToolbar />);
    await user.click(screen.getByTitle('Switch workspace'));
    expect(screen.getByText('Saved WS')).toBeInTheDocument();
  });

  it('shows "No saved workspaces" when switcher dropdown has no workspaces', async () => {
    useMultiviewStore.setState({ workspaces: [] });
    const { user } = renderWithProviders(<MultiviewToolbar />);
    await user.click(screen.getByTitle('Switch workspace'));
    expect(screen.getByText('No saved workspaces')).toBeInTheDocument();
  });

  it('calls loadWorkspace when a workspace is selected from switcher', async () => {
    const loadWorkspace = vi.fn();
    useMultiviewStore.setState({
      workspaces: [makeWorkspace({ id: 'ws-2', name: 'Other WS' })],
      loadWorkspace,
    } as never);
    const { user } = renderWithProviders(<MultiviewToolbar />);
    await user.click(screen.getByTitle('Switch workspace'));
    await user.click(screen.getByText('Other WS'));
    expect(loadWorkspace).toHaveBeenCalledWith('ws-2');
  });

  it('enters editing mode when workspace name is clicked', async () => {
    const { user } = renderWithProviders(<MultiviewToolbar />);
    await user.click(screen.getByText('My Workspace'));
    expect(screen.getByDisplayValue('My Workspace')).toBeInTheDocument();
  });

  it('calls renameWorkspace on blur from name input', async () => {
    const renameWorkspace = vi.fn();
    useMultiviewStore.setState({ renameWorkspace } as never);
    const { user } = renderWithProviders(<MultiviewToolbar />);
    await user.click(screen.getByText('My Workspace'));
    const input = screen.getByDisplayValue('My Workspace');
    await user.clear(input);
    await user.type(input, 'Renamed');
    fireEvent.blur(input);
    expect(renameWorkspace).toHaveBeenCalledWith('ws-1', 'Renamed');
  });
});
