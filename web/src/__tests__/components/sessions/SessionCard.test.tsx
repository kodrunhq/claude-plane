import { describe, it, expect, vi } from 'vitest';
import { screen } from '@testing-library/react';
import { SessionCard } from '../../../components/sessions/SessionCard';
import { renderWithProviders } from '../../../test/render';
import { buildSession } from '../../../test/factories';

const defaultCallbacks = {
  onAttach: vi.fn(),
  onTerminate: vi.fn(),
  onSelect: vi.fn(),
};

function renderCard(overrides?: Parameters<typeof buildSession>[0], props?: Record<string, unknown>) {
  const session = buildSession(overrides);
  return renderWithProviders(
    <SessionCard session={session} {...defaultCallbacks} {...props} />,
  );
}

describe('SessionCard', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  // ── Bug regression: machine ID was truncated with .slice(0, 8) ────────────
  // "ubuntu-nuc1" was displayed as "ubuntu-n". Machine IDs are human-readable
  // names (not UUIDs) so they must be shown in full.
  it('displays machine ID in full without truncation', () => {
    renderCard({ machine_id: 'ubuntu-nuc1' });
    expect(screen.getByText('ubuntu-nuc1')).toBeDefined();
  });

  it('shows machine ID with title attribute for tooltip', () => {
    renderCard({ machine_id: 'my-long-machine-name' });
    const el = screen.getByTitle('my-long-machine-name');
    expect(el.textContent).toBe('my-long-machine-name');
  });

  // Session IDs are UUIDs — truncating to 8 chars is acceptable and expected.
  it('displays session ID truncated to 8 characters', () => {
    const uuid = 'abcdef01-2345-6789-abcd-ef0123456789';
    renderCard({ session_id: uuid });
    const el = screen.getByTitle(uuid);
    expect(el.textContent).toBe('abcdef01');
  });

  // ── Click routing: selectable vs non-selectable ───────────────────────────
  it('calls onSelect (not onAttach) when clicking card in selectable mode', async () => {
    const onSelect = vi.fn();
    const onAttach = vi.fn();
    const session = buildSession({ session_id: 'sel-1' });
    const { user } = renderWithProviders(
      <SessionCard
        session={session}
        onAttach={onAttach}
        onTerminate={vi.fn()}
        selectable
        onSelect={onSelect}
      />,
    );

    await user.click(screen.getByText(session.command || 'claude'));
    expect(onSelect).toHaveBeenCalledWith('sel-1');
    expect(onAttach).not.toHaveBeenCalled();
  });

  it('calls onAttach (not onSelect) when clicking card in non-selectable mode', async () => {
    const onSelect = vi.fn();
    const onAttach = vi.fn();
    const session = buildSession({ session_id: 'att-1' });
    const { user } = renderWithProviders(
      <SessionCard
        session={session}
        onAttach={onAttach}
        onTerminate={vi.fn()}
        onSelect={onSelect}
      />,
    );

    await user.click(screen.getByText(session.command || 'claude'));
    expect(onAttach).toHaveBeenCalledWith('att-1');
    expect(onSelect).not.toHaveBeenCalled();
  });

  // ── Checkbox visibility ───────────────────────────────────────────────────
  it('renders checkbox when selectable=true', () => {
    const session = buildSession();
    renderWithProviders(
      <SessionCard session={session} {...defaultCallbacks} selectable />,
    );
    expect(screen.getByRole('checkbox')).toBeDefined();
  });

  it('does not render checkbox when selectable is not set', () => {
    renderCard();
    expect(screen.queryByRole('checkbox')).toBeNull();
  });

  // ── Terminate button ──────────────────────────────────────────────────────
  it('shows Terminate button for running sessions (not selectable)', () => {
    renderCard({ status: 'running' });
    expect(screen.getByText('Terminate')).toBeDefined();
  });

  it('does not show Terminate button for completed sessions', () => {
    renderCard({ status: 'completed' });
    expect(screen.queryByText('Terminate')).toBeNull();
  });

  it('does not show Terminate button in selectable mode even if running', () => {
    const session = buildSession({ status: 'running' });
    renderWithProviders(
      <SessionCard session={session} {...defaultCallbacks} selectable />,
    );
    expect(screen.queryByText('Terminate')).toBeNull();
  });

  // Bug regression: clicking Terminate must NOT also fire onAttach.
  it('clicking Terminate calls onTerminate without triggering onAttach', async () => {
    const onTerminate = vi.fn();
    const onAttach = vi.fn();
    const session = buildSession({ session_id: 'term-1', status: 'running' });
    const { user } = renderWithProviders(
      <SessionCard session={session} onAttach={onAttach} onTerminate={onTerminate} />,
    );

    await user.click(screen.getByText('Terminate'));
    expect(onTerminate).toHaveBeenCalledWith('term-1');
    expect(onAttach).not.toHaveBeenCalled();
  });
});
