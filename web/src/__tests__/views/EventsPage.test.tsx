import { describe, it, expect, vi } from 'vitest';
import { http, HttpResponse } from 'msw';
import { server } from '../../test/setup.ts';
import { renderWithProviders, screen, waitFor } from '../../test/render.tsx';
import { EventsPage } from '../../views/EventsPage.tsx';
import { mockEvents } from '../../test/handlers.ts';
import { buildEvent } from '../../test/factories.ts';

describe('EventsPage', () => {
  it('renders the page heading and description', async () => {
    renderWithProviders(<EventsPage />);

    expect(screen.getByRole('heading', { name: 'Event Log' })).toBeInTheDocument();
    expect(screen.getByText('Audit history of all system events')).toBeInTheDocument();
  });

  it('renders events from API data', async () => {
    renderWithProviders(<EventsPage />);

    await waitFor(() => {
      expect(screen.getByText('session.started')).toBeInTheDocument();
      expect(screen.getByText('machine.connected')).toBeInTheDocument();
    });
  });

  it('renders event type badges with correct styling', async () => {
    renderWithProviders(<EventsPage />);

    await waitFor(() => {
      const badge = screen.getByText('session.started');
      expect(badge).toBeInTheDocument();
      // Badge has font-mono class as part of a span
      expect(badge.tagName.toLowerCase()).toBe('span');
    });
  });

  it('shows table column headers', async () => {
    renderWithProviders(<EventsPage />);

    await waitFor(() => {
      expect(screen.getByText('Event Type')).toBeInTheDocument();
      expect(screen.getByText('Timestamp')).toBeInTheDocument();
      expect(screen.getByText('Source')).toBeInTheDocument();
      expect(screen.getByText('Payload')).toBeInTheDocument();
    });
  });

  it('shows empty state when no events exist', async () => {
    server.use(
      http.get('/api/v1/events', () => HttpResponse.json([])),
    );

    renderWithProviders(<EventsPage />);

    await waitFor(() => {
      expect(screen.getByText('No events found')).toBeInTheDocument();
    });

    expect(screen.getByText('No events have been recorded yet.')).toBeInTheDocument();
  });

  it('shows filter-specific empty state when filters are active', async () => {
    server.use(
      http.get('/api/v1/events', () => HttpResponse.json([])),
    );

    const { user } = renderWithProviders(<EventsPage />);

    // Type a filter value
    const typeInput = screen.getByPlaceholderText('Filter by event type (e.g. run.*)');
    await user.type(typeInput, 'nonexistent.*');

    await waitFor(() => {
      expect(screen.getByText('No events found')).toBeInTheDocument();
      expect(screen.getByText('Try adjusting your filters to see more events.')).toBeInTheDocument();
    });
  });

  it('shows error state with retry button when API fails', async () => {
    server.use(
      http.get('/api/v1/events', () =>
        HttpResponse.json({ error: 'Server error' }, { status: 500 }),
      ),
    );

    renderWithProviders(<EventsPage />);

    await waitFor(() => {
      expect(screen.getByText('Retry')).toBeInTheDocument();
    });
  });

  it('renders event type filter input', () => {
    renderWithProviders(<EventsPage />);

    expect(screen.getByPlaceholderText('Filter by event type (e.g. run.*)')).toBeInTheDocument();
  });

  it('renders since date filter', () => {
    renderWithProviders(<EventsPage />);

    expect(screen.getByText('Since')).toBeInTheDocument();
  });

  it('renders per page selector', () => {
    renderWithProviders(<EventsPage />);

    expect(screen.getByText('Per page')).toBeInTheDocument();
    expect(screen.getByDisplayValue('25')).toBeInTheDocument();
  });

  it('renders pagination info and navigation buttons', async () => {
    renderWithProviders(<EventsPage />);

    await waitFor(() => {
      expect(screen.getByText('session.started')).toBeInTheDocument();
    });

    expect(screen.getByText('Prev')).toBeInTheDocument();
    expect(screen.getByText('Next')).toBeInTheDocument();
    expect(screen.getByText(/Page 1/)).toBeInTheDocument();
  });

  it('disables prev button on first page', async () => {
    renderWithProviders(<EventsPage />);

    await waitFor(() => {
      expect(screen.getByText('session.started')).toBeInTheDocument();
    });

    const prevButton = screen.getByText('Prev').closest('button')!;
    expect(prevButton).toBeDisabled();
  });

  it('disables next button when fewer results than limit', async () => {
    // mockEvents has only 2 items, limit is 25 by default
    renderWithProviders(<EventsPage />);

    await waitFor(() => {
      expect(screen.getByText('session.started')).toBeInTheDocument();
    });

    const nextButton = screen.getByText('Next').closest('button')!;
    expect(nextButton).toBeDisabled();
  });

  it('enables next button when results fill the page', async () => {
    // Return exactly 25 events to signal there might be more
    const fullPage = Array.from({ length: 25 }, (_, i) =>
      buildEvent({ event_id: `evt-${300 + i}`, event_type: `test.event.${i}` }),
    );

    server.use(
      http.get('/api/v1/events', () => HttpResponse.json(fullPage)),
    );

    renderWithProviders(<EventsPage />);

    await waitFor(() => {
      expect(screen.getByText('test.event.0')).toBeInTheDocument();
    });

    const nextButton = screen.getByText('Next').closest('button')!;
    expect(nextButton).not.toBeDisabled();
  });

  it('renders refresh button', () => {
    renderWithProviders(<EventsPage />);

    expect(screen.getByRole('button', { name: /refresh/i })).toBeInTheDocument();
  });

  it('shows event source in the table', async () => {
    renderWithProviders(<EventsPage />);

    await waitFor(() => {
      // mockEvents have source: 'test'
      const sourceCells = screen.getAllByText('test');
      expect(sourceCells.length).toBeGreaterThan(0);
    });
  });

  it('allows changing per page limit', async () => {
    const { user } = renderWithProviders(<EventsPage />);

    const perPageSelect = screen.getByDisplayValue('25');
    await user.selectOptions(perPageSelect, '50');

    expect(perPageSelect).toHaveValue('50');
  });
});
