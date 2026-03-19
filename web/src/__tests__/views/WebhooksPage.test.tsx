import { describe, it, expect } from 'vitest';
import { http, HttpResponse } from 'msw';
import { server } from '../../test/setup.ts';
import { renderWithProviders, screen, waitFor } from '../../test/render.tsx';
import { WebhooksPage } from '../../views/WebhooksPage.tsx';
import type { Webhook } from '../../types/webhook.ts';

const mockWebhooks: Webhook[] = [
  {
    webhook_id: 'wh-100',
    name: 'Deploy Notifier',
    url: 'https://example.com/webhook',
    events: ['run.completed', 'run.failed'],
    enabled: true,
    created_at: '2026-01-15T10:00:00Z',
    updated_at: '2026-01-15T10:00:00Z',
  },
  {
    webhook_id: 'wh-101',
    name: 'Slack Alerts',
    url: 'https://hooks.slack.com/services/test',
    events: ['session.started'],
    enabled: false,
    created_at: '2026-01-14T08:00:00Z',
    updated_at: '2026-01-14T08:00:00Z',
  },
];

function setupWebhooksHandler(webhooks: Webhook[] = mockWebhooks) {
  server.use(
    http.get('/api/v1/webhooks', () => HttpResponse.json(webhooks)),
  );
}

describe('WebhooksPage', () => {
  it('renders the page heading', async () => {
    setupWebhooksHandler();
    renderWithProviders(<WebhooksPage />);

    expect(screen.getByRole('heading', { name: 'Webhooks' })).toBeInTheDocument();
  });

  it('renders webhooks list from API data', async () => {
    setupWebhooksHandler();
    renderWithProviders(<WebhooksPage />);

    await waitFor(() => {
      expect(screen.getByText('Deploy Notifier')).toBeInTheDocument();
      expect(screen.getByText('Slack Alerts')).toBeInTheDocument();
    });
  });

  it('shows loading skeleton while fetching', () => {
    server.use(
      http.get('/api/v1/webhooks', async () => {
        await new Promise((r) => setTimeout(r, 5000));
        return HttpResponse.json(mockWebhooks);
      }),
    );

    renderWithProviders(<WebhooksPage />);

    expect(screen.queryByText('No webhooks yet')).not.toBeInTheDocument();
    expect(screen.queryByText('Deploy Notifier')).not.toBeInTheDocument();
  });

  it('shows empty state when no webhooks exist', async () => {
    setupWebhooksHandler([]);
    renderWithProviders(<WebhooksPage />);

    await waitFor(() => {
      expect(screen.getByText('No webhooks yet')).toBeInTheDocument();
    });

    expect(
      screen.getByText('Create a webhook to receive real-time events when things happen in claude-plane.'),
    ).toBeInTheDocument();
  });

  it('shows error state with retry button when API fails', async () => {
    server.use(
      http.get('/api/v1/webhooks', () =>
        HttpResponse.json({ error: 'Server error' }, { status: 500 }),
      ),
    );

    renderWithProviders(<WebhooksPage />);

    await waitFor(() => {
      expect(screen.getByText('Retry')).toBeInTheDocument();
    });
  });

  it('renders New Webhook button in header', () => {
    setupWebhooksHandler();
    renderWithProviders(<WebhooksPage />);

    expect(screen.getByText('New Webhook')).toBeInTheDocument();
  });

  it('opens create webhook drawer when New Webhook button is clicked', async () => {
    setupWebhooksHandler();
    const { user } = renderWithProviders(<WebhooksPage />);

    await waitFor(() => {
      expect(screen.getByText('Deploy Notifier')).toBeInTheDocument();
    });

    // Click New Webhook in the header (not the empty state one)
    const buttons = screen.getAllByText('New Webhook');
    await user.click(buttons[0]);

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: 'New Webhook' })).toBeInTheDocument();
    });
  });

  it('opens create webhook drawer from empty state action button', async () => {
    setupWebhooksHandler([]);
    const { user } = renderWithProviders(<WebhooksPage />);

    await waitFor(() => {
      expect(screen.getByText('No webhooks yet')).toBeInTheDocument();
    });

    // The empty state has a New Webhook button as action (header also has one)
    const newButtons = screen.getAllByText('New Webhook');
    await user.click(newButtons[newButtons.length - 1]);

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: 'New Webhook' })).toBeInTheDocument();
    });
  });

  it('closes create drawer when close button is clicked', async () => {
    setupWebhooksHandler();
    const { user } = renderWithProviders(<WebhooksPage />);

    await waitFor(() => {
      expect(screen.getByText('Deploy Notifier')).toBeInTheDocument();
    });

    // Open the drawer
    const buttons = screen.getAllByText('New Webhook');
    await user.click(buttons[0]);

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: 'New Webhook' })).toBeInTheDocument();
    });

    // Click the close button (the x)
    const closeButton = screen.getByRole('button', { name: 'Close' });
    await user.click(closeButton);

    await waitFor(() => {
      expect(screen.queryByRole('heading', { name: 'New Webhook' })).not.toBeInTheDocument();
    });
  });

  it('renders the documentation link', () => {
    setupWebhooksHandler();
    renderWithProviders(<WebhooksPage />);

    expect(screen.getByText('Learn about webhook signing and integrations')).toBeInTheDocument();
  });

  it('renders webhook form inside create drawer without auto-submitting', async () => {
    let createCalled = false;
    setupWebhooksHandler();
    server.use(
      http.post('/api/v1/webhooks', async ({ request }) => {
        createCalled = true;
        const body = await request.json() as Record<string, unknown>;
        return HttpResponse.json(
          {
            webhook_id: 'wh-new',
            name: body.name,
            url: body.url,
            events: body.events,
            enabled: body.enabled,
            created_at: '2026-01-15T12:00:00Z',
            updated_at: '2026-01-15T12:00:00Z',
          },
          { status: 201 },
        );
      }),
    );

    const { user } = renderWithProviders(<WebhooksPage />);

    await waitFor(() => {
      expect(screen.getByText('Deploy Notifier')).toBeInTheDocument();
    });

    // Open the create drawer
    const buttons = screen.getAllByText('New Webhook');
    await user.click(buttons[0]);

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: 'New Webhook' })).toBeInTheDocument();
    });

    // The WebhookForm should be visible in the drawer but not yet submitted
    expect(createCalled).toBe(false);
  });
});
