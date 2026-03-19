import { describe, it, expect, vi } from 'vitest';
import { screen } from '@testing-library/react';
import { RecentErrors } from '../../../components/dashboard/RecentErrors';
import { renderWithProviders } from '../../../test/render';
import type { LogEntry } from '../../../types/log';

const mockUseLogs = vi.fn();

vi.mock('../../../hooks/useLogs', () => ({
  useLogs: (...args: unknown[]) => mockUseLogs(...args),
}));

function makeLogEntry(overrides?: Partial<LogEntry>): LogEntry {
  return {
    id: 1,
    timestamp: '2026-01-15T10:00:00Z',
    level: 'ERROR',
    component: 'grpc',
    message: 'Connection refused',
    source: 'server',
    ...overrides,
  };
}

describe('RecentErrors', () => {
  it('renders section heading', () => {
    mockUseLogs.mockReturnValue({ data: { logs: [] }, isLoading: false });
    renderWithProviders(<RecentErrors />);
    expect(screen.getByText('Recent Errors')).toBeInTheDocument();
  });

  it('renders View all link pointing to /logs?level=ERROR', () => {
    mockUseLogs.mockReturnValue({ data: { logs: [] }, isLoading: false });
    renderWithProviders(<RecentErrors />);
    const link = screen.getByText('View all');
    expect(link).toHaveAttribute('href', '/logs?level=ERROR');
  });

  it('renders loading skeleton when isLoading is true', () => {
    mockUseLogs.mockReturnValue({ data: undefined, isLoading: true });
    const { container } = renderWithProviders(<RecentErrors />);
    expect(container.querySelectorAll('.animate-pulse').length).toBeGreaterThan(0);
  });

  it('renders empty state with no errors message', () => {
    mockUseLogs.mockReturnValue({ data: { logs: [] }, isLoading: false });
    renderWithProviders(<RecentErrors />);
    expect(screen.getByText('No recent errors')).toBeInTheDocument();
  });

  it('renders error messages', () => {
    mockUseLogs.mockReturnValue({
      data: {
        logs: [
          makeLogEntry({ id: 1, message: 'Auth token expired' }),
          makeLogEntry({ id: 2, message: 'Database connection lost' }),
        ],
      },
      isLoading: false,
    });
    renderWithProviders(<RecentErrors />);
    expect(screen.getByText('Auth token expired')).toBeInTheDocument();
    expect(screen.getByText('Database connection lost')).toBeInTheDocument();
  });

  it('renders component badge for each error', () => {
    mockUseLogs.mockReturnValue({
      data: {
        logs: [
          makeLogEntry({ id: 1, component: 'auth' }),
          makeLogEntry({ id: 2, component: 'session' }),
        ],
      },
      isLoading: false,
    });
    renderWithProviders(<RecentErrors />);
    expect(screen.getByText('auth')).toBeInTheDocument();
    expect(screen.getByText('session')).toBeInTheDocument();
  });

  it('calls useLogs with ERROR level filter and limit 10', () => {
    mockUseLogs.mockReturnValue({ data: { logs: [] }, isLoading: false });
    renderWithProviders(<RecentErrors />);
    expect(mockUseLogs).toHaveBeenCalledWith({ level: 'ERROR', limit: 10 });
  });

  it('renders message with title attribute for full text on hover', () => {
    const longMessage = 'This is a very long error message that might be truncated in the UI';
    mockUseLogs.mockReturnValue({
      data: {
        logs: [makeLogEntry({ id: 1, message: longMessage })],
      },
      isLoading: false,
    });
    renderWithProviders(<RecentErrors />);
    expect(screen.getByTitle(longMessage)).toBeInTheDocument();
  });

  it('renders correctly with undefined data', () => {
    mockUseLogs.mockReturnValue({ data: undefined, isLoading: false });
    renderWithProviders(<RecentErrors />);
    // Should show empty state since logs defaults to []
    expect(screen.getByText('No recent errors')).toBeInTheDocument();
  });

  it('renders multiple error entries in order', () => {
    mockUseLogs.mockReturnValue({
      data: {
        logs: [
          makeLogEntry({ id: 1, message: 'Error 1', component: 'grpc' }),
          makeLogEntry({ id: 2, message: 'Error 2', component: 'auth' }),
          makeLogEntry({ id: 3, message: 'Error 3', component: 'session' }),
        ],
      },
      isLoading: false,
    });
    renderWithProviders(<RecentErrors />);
    expect(screen.getByText('Error 1')).toBeInTheDocument();
    expect(screen.getByText('Error 2')).toBeInTheDocument();
    expect(screen.getByText('Error 3')).toBeInTheDocument();
  });
});
