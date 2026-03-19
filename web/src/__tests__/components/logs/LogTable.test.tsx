import { describe, it, expect, vi } from 'vitest';
import { screen } from '@testing-library/react';
import { LogTable } from '../../../components/logs/LogTable';
import { renderWithProviders } from '../../../test/render';
import type { LogEntry } from '../../../types/log';

function makeEntry(overrides?: Partial<LogEntry>): LogEntry {
  return {
    id: 1,
    timestamp: new Date().toISOString(),
    level: 'INFO',
    component: 'grpc',
    message: 'Connection established',
    source: 'server',
    ...overrides,
  };
}

describe('LogTable', () => {
  it('returns null when loading is true', () => {
    const { container } = renderWithProviders(
      <LogTable entries={[]} loading={true} />,
    );
    expect(container.innerHTML).toBe('');
  });

  it('returns null when entries array is empty', () => {
    const { container } = renderWithProviders(
      <LogTable entries={[]} loading={false} />,
    );
    expect(container.innerHTML).toBe('');
  });

  it('renders table headers when entries are present', () => {
    renderWithProviders(
      <LogTable entries={[makeEntry()]} loading={false} />,
    );
    expect(screen.getByText('Time')).toBeInTheDocument();
    expect(screen.getByText('Level')).toBeInTheDocument();
    expect(screen.getByText('Source')).toBeInTheDocument();
    expect(screen.getByText('Component')).toBeInTheDocument();
    expect(screen.getByText('Message')).toBeInTheDocument();
    expect(screen.getByText('Machine')).toBeInTheDocument();
  });

  it('renders log entry data', () => {
    const entry = makeEntry({
      id: 10,
      level: 'ERROR',
      component: 'auth',
      message: 'Authentication failed',
      source: 'server',
    });
    renderWithProviders(<LogTable entries={[entry]} loading={false} />);
    expect(screen.getByText('ERROR')).toBeInTheDocument();
    expect(screen.getByText('auth')).toBeInTheDocument();
    expect(screen.getByText('Authentication failed')).toBeInTheDocument();
    expect(screen.getByText('server')).toBeInTheDocument();
  });

  it('renders level badges with correct text', () => {
    const entries = [
      makeEntry({ id: 1, level: 'DEBUG' }),
      makeEntry({ id: 2, level: 'INFO' }),
      makeEntry({ id: 3, level: 'WARN' }),
      makeEntry({ id: 4, level: 'ERROR' }),
    ];
    renderWithProviders(<LogTable entries={entries} loading={false} />);
    expect(screen.getByText('DEBUG')).toBeInTheDocument();
    expect(screen.getByText('INFO')).toBeInTheDocument();
    expect(screen.getByText('WARN')).toBeInTheDocument();
    expect(screen.getByText('ERROR')).toBeInTheDocument();
  });

  // Bug regression: machine_id was truncated in a previous version.
  // Machine IDs must be displayed in full.
  it('displays full machine_id without truncation', () => {
    const entry = makeEntry({
      id: 1,
      machine_id: 'ubuntu-nuc1-long-name',
    });
    renderWithProviders(<LogTable entries={[entry]} loading={false} />);
    expect(screen.getByText('ubuntu-nuc1-long-name')).toBeInTheDocument();
  });

  it('displays em dash when machine_id is absent', () => {
    const entry = makeEntry({ id: 1, machine_id: undefined });
    renderWithProviders(<LogTable entries={[entry]} loading={false} />);
    expect(screen.getByText('\u2014')).toBeInTheDocument();
  });

  it('renders relative time for timestamp', () => {
    const recentTimestamp = new Date(Date.now() - 30000).toISOString(); // 30s ago
    const entry = makeEntry({ id: 1, timestamp: recentTimestamp });
    renderWithProviders(<LogTable entries={[entry]} loading={false} />);
    expect(screen.getByText(/\d+s ago/)).toBeInTheDocument();
  });

  it('shows expand icon for entries with metadata', () => {
    const entry = makeEntry({
      id: 1,
      metadata: '{"key": "value"}',
    });
    renderWithProviders(<LogTable entries={[entry]} loading={false} />);
    // Row should have role="button" indicating it is expandable
    const row = screen.getByRole('button');
    expect(row).toBeInTheDocument();
  });

  it('does not show expand icon for entries without details', () => {
    const entry = makeEntry({
      id: 1,
      metadata: undefined,
      session_id: undefined,
      error: undefined,
    });
    renderWithProviders(<LogTable entries={[entry]} loading={false} />);
    expect(screen.queryByRole('button')).not.toBeInTheDocument();
  });

  it('expands row on click to show session_id', async () => {
    const entry = makeEntry({
      id: 1,
      session_id: 'sess-abc-123',
    });
    const { user } = renderWithProviders(
      <LogTable entries={[entry]} loading={false} />,
    );
    await user.click(screen.getByRole('button'));
    expect(screen.getByText('sess-abc-123')).toBeInTheDocument();
    expect(screen.getByText('Session ID:')).toBeInTheDocument();
  });

  it('expands row on click to show error details', async () => {
    const entry = makeEntry({
      id: 1,
      error: 'timeout exceeded',
    });
    const { user } = renderWithProviders(
      <LogTable entries={[entry]} loading={false} />,
    );
    await user.click(screen.getByRole('button'));
    expect(screen.getByText('timeout exceeded')).toBeInTheDocument();
    expect(screen.getByText('Error:')).toBeInTheDocument();
  });

  it('expands row to show parsed metadata JSON', async () => {
    const entry = makeEntry({
      id: 1,
      metadata: '{"request_id": "req-999"}',
    });
    const { user } = renderWithProviders(
      <LogTable entries={[entry]} loading={false} />,
    );
    await user.click(screen.getByRole('button'));
    expect(screen.getByText('Metadata:')).toBeInTheDocument();
    expect(screen.getByText(/"request_id": "req-999"/)).toBeInTheDocument();
  });

  it('collapses expanded row when clicked again', async () => {
    const entry = makeEntry({
      id: 1,
      session_id: 'sess-toggle',
    });
    const { user } = renderWithProviders(
      <LogTable entries={[entry]} loading={false} />,
    );
    await user.click(screen.getByRole('button'));
    expect(screen.getByText('sess-toggle')).toBeInTheDocument();
    await user.click(screen.getByRole('button'));
    expect(screen.queryByText('Session ID:')).not.toBeInTheDocument();
  });

  it('supports keyboard expansion with Enter key', async () => {
    const entry = makeEntry({
      id: 1,
      session_id: 'sess-keyboard',
    });
    renderWithProviders(<LogTable entries={[entry]} loading={false} />);
    const row = screen.getByRole('button');
    row.focus();
    const { fireEvent } = await import('@testing-library/react');
    fireEvent.keyDown(row, { key: 'Enter' });
    expect(screen.getByText('sess-keyboard')).toBeInTheDocument();
  });

  it('sets aria-expanded attribute on expandable rows', () => {
    const entry = makeEntry({
      id: 1,
      metadata: '{"a": 1}',
    });
    renderWithProviders(<LogTable entries={[entry]} loading={false} />);
    const row = screen.getByRole('button');
    expect(row).toHaveAttribute('aria-expanded', 'false');
  });

  it('renders multiple log entries', () => {
    const entries = [
      makeEntry({ id: 1, message: 'First message' }),
      makeEntry({ id: 2, message: 'Second message' }),
      makeEntry({ id: 3, message: 'Third message' }),
    ];
    renderWithProviders(<LogTable entries={entries} loading={false} />);
    expect(screen.getByText('First message')).toBeInTheDocument();
    expect(screen.getByText('Second message')).toBeInTheDocument();
    expect(screen.getByText('Third message')).toBeInTheDocument();
  });
});
