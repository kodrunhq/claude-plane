import { screen } from '@testing-library/react';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { MachineCard } from '../../../components/machines/MachineCard.tsx';
import { renderWithProviders } from '../../../test/render.tsx';
import { buildMachine } from '../../../test/factories.ts';
import { useAuthStore } from '../../../stores/auth.ts';
import type { Machine, MachineHealth } from '../../../lib/types.ts';

// Mock the mutation hooks
vi.mock('../../../hooks/useMachines.ts', () => ({
  useUpdateMachine: () => ({
    mutate: vi.fn(),
    isPending: false,
  }),
  useDeleteMachine: () => ({
    mutate: vi.fn(),
    isPending: false,
  }),
}));

const defaultHealth: MachineHealth = {
  cpu_cores: 8,
  memory_total_mb: 16384,
  memory_used_mb: 8192,
  active_sessions: 2,
  max_sessions: 5,
};

function renderCard(machineOverrides?: Partial<Machine>, onCreateSession = vi.fn()) {
  const machine = buildMachine(machineOverrides);
  return {
    machine,
    onCreateSession,
    ...renderWithProviders(
      <MachineCard machine={machine} onCreateSession={onCreateSession} />,
    ),
  };
}

describe('MachineCard', () => {
  beforeEach(() => {
    // Default to non-admin user
    useAuthStore.setState({
      user: { userId: 'u1', email: 'user@test.com', displayName: 'User', role: 'user' },
      authenticated: true,
      loading: false,
    });
  });

  describe('machine identification', () => {
    it('displays the full machine_id (not truncated)', () => {
      const { machine } = renderCard({
        machine_id: 'very-long-machine-identifier-abcdef1234567890',
      });
      // The machine_id should appear in full via the title attribute and as text
      expect(screen.getByTitle('very-long-machine-identifier-abcdef1234567890')).toBeInTheDocument();
      expect(screen.getByText('very-long-machine-identifier-abcdef1234567890')).toBeInTheDocument();
    });

    it('displays display_name when available', () => {
      renderCard({ display_name: 'Production Worker' });
      expect(screen.getByText('Production Worker')).toBeInTheDocument();
    });

    it('falls back to machine_id when display_name is empty', () => {
      const { machine } = renderCard({ display_name: '' });
      // machine_id appears both in the display name <p> and in the machine_id <span>,
      // so use getAllByText and verify there are at least 2 occurrences (fallback + ID line)
      const elements = screen.getAllByText(machine.machine_id);
      expect(elements.length).toBeGreaterThanOrEqual(2);
    });
  });

  describe('status indicator', () => {
    it('renders status badge for connected machine', () => {
      renderCard({ status: 'connected' });
      expect(screen.getByText('connected')).toBeInTheDocument();
    });

    it('renders status badge for disconnected machine', () => {
      renderCard({ status: 'disconnected' });
      expect(screen.getByText('disconnected')).toBeInTheDocument();
    });
  });

  describe('health metrics', () => {
    it('displays active sessions count', () => {
      renderCard({ health: defaultHealth });
      expect(screen.getByText(/2 active sessions/)).toBeInTheDocument();
    });

    it('uses singular "session" for 1 active session', () => {
      renderCard({ health: { ...defaultHealth, active_sessions: 1 } });
      expect(screen.getByText('1 active session')).toBeInTheDocument();
    });

    it('displays CPU cores', () => {
      renderCard({ health: defaultHealth });
      expect(screen.getByText('8 cores')).toBeInTheDocument();
    });

    it('displays memory usage formatted as GB', () => {
      renderCard({ health: defaultHealth });
      // 8192 MB = 8.0 GB, 16384 MB = 16.0 GB
      expect(screen.getByText(/8\.0 GB \/ 16\.0 GB/)).toBeInTheDocument();
    });

    it('displays memory percentage', () => {
      renderCard({ health: defaultHealth });
      expect(screen.getByText('(50%)')).toBeInTheDocument();
    });

    it('shows "Awaiting health data..." when connected but no health', () => {
      renderCard({ status: 'connected', health: undefined });
      expect(screen.getByText('Awaiting health data...')).toBeInTheDocument();
    });

    it('shows "Offline" when disconnected and no health', () => {
      renderCard({ status: 'disconnected', health: undefined });
      expect(screen.getByText('Offline')).toBeInTheDocument();
    });

    it('displays memory in MB when under 1024 MB', () => {
      renderCard({
        health: { ...defaultHealth, memory_total_mb: 512, memory_used_mb: 256 },
      });
      expect(screen.getByText(/256 MB \/ 512 MB/)).toBeInTheDocument();
    });
  });

  describe('New Session button', () => {
    it('renders the "New Session" button', () => {
      renderCard();
      expect(screen.getByRole('button', { name: 'New Session' })).toBeInTheDocument();
    });

    it('calls onCreateSession with machine_id when clicked', async () => {
      const onCreateSession = vi.fn();
      const { machine, user } = renderCard({ status: 'connected' }, onCreateSession);

      await user.click(screen.getByRole('button', { name: 'New Session' }));
      expect(onCreateSession).toHaveBeenCalledWith(machine.machine_id);
    });

    it('is enabled when machine is connected', () => {
      renderCard({ status: 'connected' });
      expect(screen.getByRole('button', { name: 'New Session' })).not.toBeDisabled();
    });

    it('is disabled when machine is disconnected', () => {
      renderCard({ status: 'disconnected' });
      expect(screen.getByRole('button', { name: 'New Session' })).toBeDisabled();
    });
  });

  describe('rename (edit) functionality', () => {
    it('shows the rename button on the display name row', () => {
      renderCard({ display_name: 'Worker A' });
      expect(screen.getByRole('button', { name: 'Rename machine' })).toBeInTheDocument();
    });

    it('enters editing mode when rename button is clicked', async () => {
      const { user } = renderCard({ display_name: 'Worker A' });

      await user.click(screen.getByRole('button', { name: 'Rename machine' }));
      expect(screen.getByRole('textbox')).toBeInTheDocument();
      expect(screen.getByRole('textbox')).toHaveValue('Worker A');
    });

    it('shows save and cancel buttons in editing mode', async () => {
      const { user } = renderCard({ display_name: 'Worker A' });

      await user.click(screen.getByRole('button', { name: 'Rename machine' }));
      expect(screen.getByRole('button', { name: 'Save machine name' })).toBeInTheDocument();
      expect(screen.getByRole('button', { name: 'Cancel rename' })).toBeInTheDocument();
    });

    it('exits editing mode when cancel is clicked', async () => {
      const { user } = renderCard({ display_name: 'Worker A' });

      await user.click(screen.getByRole('button', { name: 'Rename machine' }));
      await user.click(screen.getByRole('button', { name: 'Cancel rename' }));
      expect(screen.queryByRole('textbox')).not.toBeInTheDocument();
    });
  });

  describe('delete button (admin only, disconnected only)', () => {
    it('does not show delete button for non-admin users', () => {
      renderCard({ status: 'disconnected' });
      expect(screen.queryByRole('button', { name: 'Remove machine' })).not.toBeInTheDocument();
    });

    it('does not show delete button for connected machines even as admin', () => {
      useAuthStore.setState({
        user: { userId: 'u1', email: 'admin@test.com', displayName: 'Admin', role: 'admin' },
      });
      renderCard({ status: 'connected' });
      expect(screen.queryByRole('button', { name: 'Remove machine' })).not.toBeInTheDocument();
    });

    it('shows delete button for admin on disconnected machine', () => {
      useAuthStore.setState({
        user: { userId: 'u1', email: 'admin@test.com', displayName: 'Admin', role: 'admin' },
      });
      renderCard({ status: 'disconnected' });
      expect(screen.getByRole('button', { name: 'Remove machine' })).toBeInTheDocument();
    });

    it('opens confirm dialog when delete button is clicked', async () => {
      useAuthStore.setState({
        user: { userId: 'u1', email: 'admin@test.com', displayName: 'Admin', role: 'admin' },
      });
      const { user } = renderCard({ status: 'disconnected', display_name: 'Old Worker' });

      await user.click(screen.getByRole('button', { name: 'Remove machine' }));
      expect(screen.getByText('Remove Machine')).toBeInTheDocument();
      expect(screen.getByText(/Are you sure you want to remove "Old Worker"/)).toBeInTheDocument();
    });
  });
});
