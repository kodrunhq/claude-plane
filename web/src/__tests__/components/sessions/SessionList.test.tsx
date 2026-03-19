import { describe, it, expect, vi } from 'vitest';
import { renderWithProviders, screen } from '../../../test/render.tsx';
import { SessionList } from '../../../components/sessions/SessionList.tsx';
import { buildSession } from '../../../test/factories.ts';

const sessions = [
  buildSession({ session_id: 'sess-aaa11111', status: 'running', command: 'claude' }),
  buildSession({ session_id: 'sess-bbb22222', status: 'completed', command: 'claude' }),
  buildSession({ session_id: 'sess-ccc33333', status: 'running', command: 'bash' }),
];

describe('SessionList', () => {
  it('renders correct number of session cards', () => {
    renderWithProviders(
      <SessionList
        sessions={sessions}
        onAttach={vi.fn()}
        onTerminate={vi.fn()}
      />,
    );

    // Each SessionCard renders the first 8 chars of session_id
    for (const session of sessions) {
      expect(screen.getByText(session.session_id.slice(0, 8))).toBeInTheDocument();
    }
  });

  it('shows empty state when sessions array is empty', () => {
    renderWithProviders(
      <SessionList
        sessions={[]}
        onAttach={vi.fn()}
        onTerminate={vi.fn()}
      />,
    );

    expect(screen.getByText('No sessions')).toBeInTheDocument();
    expect(
      screen.getByText('No sessions found. Create a new session to get started.'),
    ).toBeInTheDocument();
  });

  it('shows custom empty message when provided', () => {
    renderWithProviders(
      <SessionList
        sessions={[]}
        onAttach={vi.fn()}
        onTerminate={vi.fn()}
        emptyMessage="Nothing here yet"
      />,
    );

    expect(screen.getByText('Nothing here yet')).toBeInTheDocument();
  });

  it('passes selectable/selectedIds/onSelect to each card — checkboxes appear in selectable mode', () => {
    const onSelect = vi.fn();
    const selectedIds = new Set(['sess-aaa11111']);

    renderWithProviders(
      <SessionList
        sessions={sessions}
        onAttach={vi.fn()}
        onTerminate={vi.fn()}
        selectable={true}
        selectedIds={selectedIds}
        onSelect={onSelect}
      />,
    );

    // In selectable mode, each card renders a checkbox with aria-label
    const checkboxes = screen.getAllByRole('checkbox');
    expect(checkboxes).toHaveLength(sessions.length);

    // The first session should be checked (it's in selectedIds)
    const firstCheckbox = screen.getByLabelText(`Select session ${sessions[0].session_id.slice(0, 8)}`);
    expect(firstCheckbox).toBeChecked();

    // The second session should not be checked
    const secondCheckbox = screen.getByLabelText(`Select session ${sessions[1].session_id.slice(0, 8)}`);
    expect(secondCheckbox).not.toBeChecked();
  });

  it('cards are clickable in selectable mode and call onSelect', async () => {
    const onSelect = vi.fn();
    const onAttach = vi.fn();

    const { user } = renderWithProviders(
      <SessionList
        sessions={sessions}
        onAttach={onAttach}
        onTerminate={vi.fn()}
        selectable={true}
        selectedIds={new Set()}
        onSelect={onSelect}
      />,
    );

    // Click the card area (the session_id text is inside the card div)
    const cardText = screen.getByText(sessions[0].session_id.slice(0, 8));
    const card = cardText.closest('.gradient-border-card')!;
    await user.click(card);

    // In selectable mode, clicking the card calls onSelect, not onAttach
    expect(onSelect).toHaveBeenCalledWith(sessions[0].session_id);
    expect(onAttach).not.toHaveBeenCalled();
  });

  it('cards call onAttach when not in selectable mode', async () => {
    const onAttach = vi.fn();
    const onSelect = vi.fn();

    const { user } = renderWithProviders(
      <SessionList
        sessions={sessions}
        onAttach={onAttach}
        onTerminate={vi.fn()}
        selectable={false}
        onSelect={onSelect}
      />,
    );

    const cardText = screen.getByText(sessions[0].session_id.slice(0, 8));
    const card = cardText.closest('.gradient-border-card')!;
    await user.click(card);

    // In non-selectable mode, clicking the card calls onAttach
    expect(onAttach).toHaveBeenCalledWith(sessions[0].session_id);
    expect(onSelect).not.toHaveBeenCalled();
  });
});
