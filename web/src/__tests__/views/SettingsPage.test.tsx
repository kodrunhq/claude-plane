import { describe, it, expect, vi } from 'vitest';
import { http, HttpResponse } from 'msw';
import { server } from '../../test/setup.ts';
import { renderWithProviders, screen, waitFor } from '../../test/render.tsx';
import { SettingsPage } from '../../views/SettingsPage.tsx';
import type { UserPreferences } from '../../types/preferences.ts';

// Mock the AccountTab since it depends on auth store state
vi.mock('../../components/settings/AccountTab.tsx', () => ({
  AccountTab: () => <div data-testid="account-tab">Account Settings Content</div>,
}));

// Mock sub-tabs that have complex dependencies
vi.mock('../../components/settings/SessionDefaultsTab.tsx', () => ({
  SessionDefaultsTab: () => <div data-testid="session-defaults-tab">Session Defaults Content</div>,
}));

vi.mock('../../components/settings/JobDefaultsTab.tsx', () => ({
  JobDefaultsTab: () => <div data-testid="job-defaults-tab">Job Defaults Content</div>,
}));

vi.mock('../../components/settings/NotificationsTab.tsx', () => ({
  NotificationsTab: () => <div data-testid="notifications-tab">Notifications Content</div>,
}));

vi.mock('../../components/settings/UIPreferencesTab.tsx', () => ({
  UIPreferencesTab: () => <div data-testid="ui-prefs-tab">UI Preferences Content</div>,
}));

vi.mock('../../components/settings/MachinesTab.tsx', () => ({
  MachinesTab: () => <div data-testid="machines-tab">Machines Settings Content</div>,
}));

const mockPreferences: UserPreferences = {
  ui: {
    theme: 'dark',
    terminal_font_size: 14,
    auto_attach_session: false,
    command_center_cards: ['sessions', 'machines', 'jobs', 'runs'],
  },
  default_session_timeout: 300,
  default_step_timeout: 600,
};

function setupPreferencesHandler(prefs: UserPreferences = mockPreferences) {
  server.use(
    http.get('/api/v1/users/me/preferences', () => HttpResponse.json(prefs)),
  );
}

function setupServerSettingsHandler() {
  server.use(
    http.get('/api/v1/settings', () =>
      HttpResponse.json({ retention_days: '30' }),
    ),
  );
}

describe('SettingsPage', () => {
  it('renders the page heading', async () => {
    setupPreferencesHandler();
    renderWithProviders(<SettingsPage />);

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: 'Settings' })).toBeInTheDocument();
    });
  });

  it('shows loading skeleton while fetching preferences', () => {
    server.use(
      http.get('/api/v1/users/me/preferences', async () => {
        await new Promise((r) => setTimeout(r, 5000));
        return HttpResponse.json(mockPreferences);
      }),
    );

    renderWithProviders(<SettingsPage />);

    // Should not show tabs or content yet
    expect(screen.queryByText('Account')).not.toBeInTheDocument();
  });

  it('shows error state with retry button when API fails', async () => {
    server.use(
      http.get('/api/v1/users/me/preferences', () =>
        HttpResponse.json({ error: 'Server error' }, { status: 500 }),
      ),
    );

    renderWithProviders(<SettingsPage />);

    await waitFor(() => {
      expect(screen.getByText('Retry')).toBeInTheDocument();
    });
  });

  it('renders all tab buttons', async () => {
    setupPreferencesHandler();
    renderWithProviders(<SettingsPage />);

    await waitFor(() => {
      expect(screen.getByText('Account')).toBeInTheDocument();
    });

    expect(screen.getByText('Session Defaults')).toBeInTheDocument();
    expect(screen.getByText('Job Defaults')).toBeInTheDocument();
    expect(screen.getByText('Notifications')).toBeInTheDocument();
    expect(screen.getByText('UI Preferences')).toBeInTheDocument();
    expect(screen.getByText('Machines')).toBeInTheDocument();
    expect(screen.getByText('Data Retention')).toBeInTheDocument();
  });

  it('shows Account tab content by default', async () => {
    setupPreferencesHandler();
    renderWithProviders(<SettingsPage />);

    await waitFor(() => {
      expect(screen.getByTestId('account-tab')).toBeInTheDocument();
    });

    expect(screen.getByText('Account Settings Content')).toBeInTheDocument();
  });

  it('switches to Session Defaults tab when clicked', async () => {
    setupPreferencesHandler();
    const { user } = renderWithProviders(<SettingsPage />);

    await waitFor(() => {
      expect(screen.getByText('Session Defaults')).toBeInTheDocument();
    });

    await user.click(screen.getByText('Session Defaults'));

    await waitFor(() => {
      expect(screen.getByTestId('session-defaults-tab')).toBeInTheDocument();
    });

    // Account tab should no longer be visible
    expect(screen.queryByTestId('account-tab')).not.toBeInTheDocument();
  });

  it('switches to Job Defaults tab when clicked', async () => {
    setupPreferencesHandler();
    const { user } = renderWithProviders(<SettingsPage />);

    await waitFor(() => {
      expect(screen.getByText('Job Defaults')).toBeInTheDocument();
    });

    await user.click(screen.getByText('Job Defaults'));

    await waitFor(() => {
      expect(screen.getByTestId('job-defaults-tab')).toBeInTheDocument();
    });
  });

  it('switches to Notifications tab when clicked', async () => {
    setupPreferencesHandler();
    const { user } = renderWithProviders(<SettingsPage />);

    await waitFor(() => {
      expect(screen.getByText('Notifications')).toBeInTheDocument();
    });

    await user.click(screen.getByText('Notifications'));

    await waitFor(() => {
      expect(screen.getByTestId('notifications-tab')).toBeInTheDocument();
    });
  });

  it('switches to UI Preferences tab when clicked', async () => {
    setupPreferencesHandler();
    const { user } = renderWithProviders(<SettingsPage />);

    await waitFor(() => {
      expect(screen.getByText('UI Preferences')).toBeInTheDocument();
    });

    await user.click(screen.getByText('UI Preferences'));

    await waitFor(() => {
      expect(screen.getByTestId('ui-prefs-tab')).toBeInTheDocument();
    });
  });

  it('switches to Machines tab when clicked', async () => {
    setupPreferencesHandler();
    const { user } = renderWithProviders(<SettingsPage />);

    await waitFor(() => {
      expect(screen.getByText('Machines')).toBeInTheDocument();
    });

    await user.click(screen.getByText('Machines'));

    await waitFor(() => {
      expect(screen.getByTestId('machines-tab')).toBeInTheDocument();
    });
  });

  it('switches to Data Retention tab when clicked', async () => {
    setupPreferencesHandler();
    setupServerSettingsHandler();
    const { user } = renderWithProviders(<SettingsPage />);

    await waitFor(() => {
      expect(screen.getByText('Data Retention')).toBeInTheDocument();
    });

    await user.click(screen.getByText('Data Retention'));

    await waitFor(() => {
      expect(screen.getByText('Retention Period')).toBeInTheDocument();
    });
  });

  it('renders Data Retention tab with retention options', async () => {
    setupPreferencesHandler();
    setupServerSettingsHandler();
    const { user } = renderWithProviders(<SettingsPage />);

    await waitFor(() => {
      expect(screen.getByText('Data Retention')).toBeInTheDocument();
    });

    await user.click(screen.getByText('Data Retention'));

    await waitFor(() => {
      expect(screen.getByText('Retention Period')).toBeInTheDocument();
    });

    // Check for retention options in the select
    expect(screen.getByText('7 days')).toBeInTheDocument();
    expect(screen.getByText('30 days')).toBeInTheDocument();
    expect(screen.getByText('90 days')).toBeInTheDocument();
    expect(screen.getByText('1 year')).toBeInTheDocument();
    expect(screen.getByText('Unlimited')).toBeInTheDocument();
  });

  it('Data Retention tab shows current retention value from server', async () => {
    setupPreferencesHandler();
    setupServerSettingsHandler();
    const { user } = renderWithProviders(<SettingsPage />);

    await waitFor(() => {
      expect(screen.getByText('Data Retention')).toBeInTheDocument();
    });

    await user.click(screen.getByText('Data Retention'));

    await waitFor(() => {
      const select = screen.getByLabelText('Retention Period');
      expect(select).toHaveValue('30');
    });
  });

  it('allows switching between multiple tabs in sequence', async () => {
    setupPreferencesHandler();
    const { user } = renderWithProviders(<SettingsPage />);

    await waitFor(() => {
      expect(screen.getByTestId('account-tab')).toBeInTheDocument();
    });

    // Switch to Sessions
    await user.click(screen.getByText('Session Defaults'));
    await waitFor(() => {
      expect(screen.getByTestId('session-defaults-tab')).toBeInTheDocument();
    });

    // Switch to Jobs
    await user.click(screen.getByText('Job Defaults'));
    await waitFor(() => {
      expect(screen.getByTestId('job-defaults-tab')).toBeInTheDocument();
    });

    // Switch back to Account
    await user.click(screen.getByText('Account'));
    await waitFor(() => {
      expect(screen.getByTestId('account-tab')).toBeInTheDocument();
    });
  });
});
