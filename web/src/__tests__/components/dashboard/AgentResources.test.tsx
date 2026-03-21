import { describe, it, expect, vi } from 'vitest';
import { screen } from '@testing-library/react';
import { AgentResources } from '../../../components/dashboard/AgentResources';
import { renderWithProviders } from '../../../test/render';
import type { Machine, MachineHealth } from '../../../lib/types';

const mockUseMachines = vi.fn();

vi.mock('../../../hooks/useMachines', () => ({
  useMachines: () => mockUseMachines(),
}));

function makeHealth(overrides?: Partial<MachineHealth>): MachineHealth {
  return {
    cpu_cores: 8,
    memory_total_mb: 16384,
    memory_used_mb: 8192,
    active_sessions: 2,
    max_sessions: 5,
    ...overrides,
  };
}

function makeMachine(overrides?: Partial<Machine>): Machine {
  return {
    machine_id: 'worker-1',
    display_name: 'Worker One',
    status: 'connected',
    max_sessions: 5,
    home_dir: '/home/worker',
    last_seen_at: '2026-01-15T10:00:00Z',
    created_at: '2026-01-01T00:00:00Z',
    health: makeHealth(),
    ...overrides,
  };
}

describe('AgentResources', () => {
  it('renders section heading', () => {
    mockUseMachines.mockReturnValue({ data: [], isLoading: false });
    renderWithProviders(<AgentResources />);
    expect(screen.getByText('Agent Resources')).toBeInTheDocument();
  });

  it('renders loading skeleton when isLoading is true', () => {
    mockUseMachines.mockReturnValue({ data: undefined, isLoading: true });
    const { container } = renderWithProviders(<AgentResources />);
    expect(container.querySelectorAll('.animate-pulse').length).toBeGreaterThan(0);
  });

  it('renders empty state when no connected agents', () => {
    mockUseMachines.mockReturnValue({ data: [], isLoading: false });
    renderWithProviders(<AgentResources />);
    expect(screen.getByText('No connected agents.')).toBeInTheDocument();
  });

  it('filters out disconnected machines', () => {
    mockUseMachines.mockReturnValue({
      data: [
        makeMachine({ machine_id: 'active', display_name: 'Active', status: 'connected' }),
        makeMachine({ machine_id: 'offline', display_name: 'Offline', status: 'disconnected' }),
      ],
      isLoading: false,
    });
    renderWithProviders(<AgentResources />);
    expect(screen.getByText('Active')).toBeInTheDocument();
    expect(screen.queryByText('Offline')).not.toBeInTheDocument();
  });

  // Machine names must be displayed in full (not truncated)
  it('displays full machine name without truncation', () => {
    mockUseMachines.mockReturnValue({
      data: [
        makeMachine({
          machine_id: 'ubuntu-nuc1-primary-workstation',
          display_name: 'ubuntu-nuc1-primary-workstation',
        }),
      ],
      isLoading: false,
    });
    renderWithProviders(<AgentResources />);
    expect(screen.getByText('ubuntu-nuc1-primary-workstation')).toBeInTheDocument();
  });

  it('falls back to machine_id when display_name is empty', () => {
    mockUseMachines.mockReturnValue({
      data: [
        makeMachine({ machine_id: 'fallback-id', display_name: '' }),
      ],
      isLoading: false,
    });
    renderWithProviders(<AgentResources />);
    expect(screen.getByText('fallback-id')).toBeInTheDocument();
  });

  it('renders Memory and Session Load resource bars', () => {
    mockUseMachines.mockReturnValue({
      data: [makeMachine()],
      isLoading: false,
    });
    renderWithProviders(<AgentResources />);
    expect(screen.getByText('Memory')).toBeInTheDocument();
    expect(screen.getByText('Session Load')).toBeInTheDocument();
  });

  it('renders correct memory percentage', () => {
    mockUseMachines.mockReturnValue({
      data: [
        makeMachine({
          health: makeHealth({ memory_used_mb: 12288, memory_total_mb: 16384 }),
        }),
      ],
      isLoading: false,
    });
    renderWithProviders(<AgentResources />);
    // 12288/16384 = 75%
    expect(screen.getByText('75%')).toBeInTheDocument();
  });

  it('renders correct session load percentage', () => {
    mockUseMachines.mockReturnValue({
      data: [
        makeMachine({
          health: makeHealth({ active_sessions: 3, max_sessions: 5 }),
        }),
      ],
      isLoading: false,
    });
    renderWithProviders(<AgentResources />);
    // 3/5 = 60%
    expect(screen.getByText('60%')).toBeInTheDocument();
  });

  it('renders CPU cores count', () => {
    mockUseMachines.mockReturnValue({
      data: [
        makeMachine({ health: makeHealth({ cpu_cores: 16 }) }),
      ],
      isLoading: false,
    });
    renderWithProviders(<AgentResources />);
    expect(screen.getByText('16 cores')).toBeInTheDocument();
  });

  it('shows no resource data message when health is undefined', () => {
    mockUseMachines.mockReturnValue({
      data: [makeMachine({ health: undefined })],
      isLoading: false,
    });
    renderWithProviders(<AgentResources />);
    expect(screen.getByText('No resource data available.')).toBeInTheDocument();
  });

  it('renders multiple connected machines', () => {
    mockUseMachines.mockReturnValue({
      data: [
        makeMachine({ machine_id: 'm1', display_name: 'Machine Alpha' }),
        makeMachine({ machine_id: 'm2', display_name: 'Machine Beta' }),
        makeMachine({ machine_id: 'm3', display_name: 'Machine Gamma' }),
      ],
      isLoading: false,
    });
    renderWithProviders(<AgentResources />);
    expect(screen.getByText('Machine Alpha')).toBeInTheDocument();
    expect(screen.getByText('Machine Beta')).toBeInTheDocument();
    expect(screen.getByText('Machine Gamma')).toBeInTheDocument();
  });

  it('handles 0% memory when total is 0', () => {
    mockUseMachines.mockReturnValue({
      data: [
        makeMachine({
          health: makeHealth({ memory_used_mb: 0, memory_total_mb: 0 }),
        }),
      ],
      isLoading: false,
    });
    renderWithProviders(<AgentResources />);
    expect(screen.getByText('0%')).toBeInTheDocument();
  });
});
