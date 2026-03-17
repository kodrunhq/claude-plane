import { describe, it, expect } from 'vitest';
import { http, HttpResponse } from 'msw';
import { server } from '../../test/setup.ts';
import { renderWithProviders, screen, waitFor } from '../../test/render.tsx';
import { CredentialsPage } from '../../views/CredentialsPage.tsx';

describe('CredentialsPage', () => {
  it('renders page heading', () => {
    server.use(
      http.get('/api/v1/credentials', () => HttpResponse.json([])),
      http.get('/api/v1/credentials/status', () =>
        HttpResponse.json({ encryption_enabled: true }),
      ),
    );

    renderWithProviders(<CredentialsPage />);

    expect(screen.getByText('Credentials')).toBeInTheDocument();
  });

  it('shows warning banner when encryption is disabled', async () => {
    server.use(
      http.get('/api/v1/credentials', () => HttpResponse.json([])),
      http.get('/api/v1/credentials/status', () =>
        HttpResponse.json({ encryption_enabled: false }),
      ),
    );

    renderWithProviders(<CredentialsPage />);

    await waitFor(() => {
      expect(
        screen.getByText(
          'Credentials are stored without encryption. Configure an encryption key for production use.',
        ),
      ).toBeInTheDocument();
    });
  });

  it('does not show warning banner when encryption is enabled', async () => {
    server.use(
      http.get('/api/v1/credentials', () => HttpResponse.json([])),
      http.get('/api/v1/credentials/status', () =>
        HttpResponse.json({ encryption_enabled: true }),
      ),
    );

    renderWithProviders(<CredentialsPage />);

    // Wait for the status query to resolve.
    await waitFor(() => {
      expect(screen.getByText('No credentials yet')).toBeInTheDocument();
    });

    expect(
      screen.queryByText(
        'Credentials are stored without encryption. Configure an encryption key for production use.',
      ),
    ).not.toBeInTheDocument();
  });

  it('shows empty state when no credentials exist', async () => {
    server.use(
      http.get('/api/v1/credentials', () => HttpResponse.json([])),
      http.get('/api/v1/credentials/status', () =>
        HttpResponse.json({ encryption_enabled: true }),
      ),
    );

    renderWithProviders(<CredentialsPage />);

    await waitFor(() => {
      expect(screen.getByText('No credentials yet')).toBeInTheDocument();
    });
  });
});
