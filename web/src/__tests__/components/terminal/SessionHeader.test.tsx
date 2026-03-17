import { describe, it, expect } from 'vitest';
import { renderWithProviders, screen } from '../../../test/render.tsx';
import { SessionHeader } from '../../../components/terminal/SessionHeader.tsx';
import { buildSession } from '../../../test/factories.ts';

const noop = () => {};

describe('SessionHeader', () => {
  it('renders loading state when session is loading', () => {
    renderWithProviders(
      <SessionHeader session={undefined} isLoading={true} onTerminate={noop} />,
    );

    expect(screen.getByText('Loading session...')).toBeInTheDocument();
  });

  it('renders session not found when no session and not loading', () => {
    renderWithProviders(
      <SessionHeader session={undefined} isLoading={false} onTerminate={noop} />,
    );

    expect(screen.getByText('Session not found')).toBeInTheDocument();
  });

  it('renders session metadata for a running session', () => {
    const session = buildSession({
      session_id: 'abcdef1234567890',
      status: 'running',
      command: 'claude --model opus',
      machine_id: 'worker-abcd1234',
      working_dir: '/home/user/project',
    });

    renderWithProviders(
      <SessionHeader session={session} isLoading={false} onTerminate={noop} />,
    );

    // Session ID truncated to 8 chars
    expect(screen.getByText('abcdef12')).toBeInTheDocument();

    // Command
    expect(screen.getByText('claude --model opus')).toBeInTheDocument();

    // Machine ID truncated
    expect(screen.getByText('worker-a')).toBeInTheDocument();

    // Working directory
    expect(screen.getByText('/home/user/project')).toBeInTheDocument();
  });

  it('renders back link pointing to /sessions', () => {
    const session = buildSession({ status: 'running' });

    renderWithProviders(
      <SessionHeader session={session} isLoading={false} onTerminate={noop} />,
    );

    const backLink = screen.getByLabelText('Back to sessions');
    expect(backLink).toBeInTheDocument();
    expect(backLink).toHaveAttribute('href', '/sessions');
  });

  it('shows terminate button for running sessions', () => {
    const session = buildSession({ status: 'running' });

    renderWithProviders(
      <SessionHeader session={session} isLoading={false} onTerminate={noop} />,
    );

    const terminateButton = screen.getByLabelText('Terminate session');
    expect(terminateButton).toBeInTheDocument();
    expect(terminateButton).toHaveAttribute('type', 'button');
  });

  it('shows terminate button for created sessions', () => {
    const session = buildSession({ status: 'created' });

    renderWithProviders(
      <SessionHeader session={session} isLoading={false} onTerminate={noop} />,
    );

    expect(screen.getByLabelText('Terminate session')).toBeInTheDocument();
  });

  it('hides terminate button for completed sessions', () => {
    const session = buildSession({ status: 'completed' });

    renderWithProviders(
      <SessionHeader session={session} isLoading={false} onTerminate={noop} />,
    );

    expect(screen.queryByLabelText('Terminate session')).not.toBeInTheDocument();
  });

  it('hides terminate button for terminated sessions', () => {
    const session = buildSession({ status: 'terminated' });

    renderWithProviders(
      <SessionHeader session={session} isLoading={false} onTerminate={noop} />,
    );

    expect(screen.queryByLabelText('Terminate session')).not.toBeInTheDocument();
  });

  it('shows model badge when model is set', () => {
    const session = buildSession({ status: 'running', model: 'opus-4' });

    renderWithProviders(
      <SessionHeader session={session} isLoading={false} onTerminate={noop} />,
    );

    const badge = screen.getByText('opus-4');
    expect(badge).toBeInTheDocument();
    expect(badge.className).toContain('bg-purple-500/10');
  });

  it('does not show model badge when model is not set', () => {
    const session = buildSession({ status: 'running', model: undefined });

    renderWithProviders(
      <SessionHeader session={session} isLoading={false} onTerminate={noop} />,
    );

    // No purple badge should exist
    const purpleBadges = document.querySelectorAll('.bg-purple-500\\/10');
    expect(purpleBadges).toHaveLength(0);
  });

  it('opens confirm dialog when terminate is clicked', async () => {
    const session = buildSession({ status: 'running' });

    const { user } = renderWithProviders(
      <SessionHeader session={session} isLoading={false} onTerminate={noop} />,
    );

    await user.click(screen.getByLabelText('Terminate session'));

    expect(screen.getByText('Terminate Session')).toBeInTheDocument();
    expect(screen.getByText(/Are you sure you want to terminate/)).toBeInTheDocument();
  });
});
