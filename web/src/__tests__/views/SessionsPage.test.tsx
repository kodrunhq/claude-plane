import { describe, it, expect } from 'vitest';
import { http, HttpResponse } from 'msw';
import { server } from '../../test/setup.ts';
import { renderWithProviders, screen, waitFor } from '../../test/render.tsx';
import { SessionsPage } from '../../views/SessionsPage.tsx';
import { mockSessions } from '../../test/handlers.ts';

describe('SessionsPage', () => {
  it('renders session cards from API data', async () => {
    renderWithProviders(<SessionsPage />);

    // Wait for mock session data to load and render
    await waitFor(() => {
      // SessionCard renders the session_id prefix (first 8 chars)
      expect(screen.getByText(mockSessions[0].session_id.slice(0, 8))).toBeInTheDocument();
    });
  });

  it('shows empty state when no sessions', async () => {
    server.use(
      http.get('/api/v1/sessions', () => HttpResponse.json([])),
    );

    renderWithProviders(<SessionsPage />);

    await waitFor(() => {
      expect(screen.getByText('No sessions')).toBeInTheDocument();
    });
  });

  it('renders status filter dropdown', async () => {
    renderWithProviders(<SessionsPage />);

    // The status select should be present with expected options
    const statusSelect = screen.getByDisplayValue('Running');
    expect(statusSelect).toBeInTheDocument();

    // Verify the label
    expect(screen.getByText('Status')).toBeInTheDocument();
  });

  it('renders page heading and New Session button', () => {
    renderWithProviders(<SessionsPage />);

    expect(screen.getByText('Sessions')).toBeInTheDocument();
    expect(screen.getByText('New Session')).toBeInTheDocument();
  });

  it('renders machine filter dropdown', async () => {
    renderWithProviders(<SessionsPage />);

    expect(screen.getByText('Machine')).toBeInTheDocument();
    expect(screen.getByDisplayValue('All Machines')).toBeInTheDocument();
  });
});
