import { describe, it, expect, vi } from 'vitest';
import { http, HttpResponse } from 'msw';
import { server } from '../../../test/setup.ts';
import { renderWithProviders, screen, waitFor } from '../../../test/render.tsx';
import { CredentialsList } from '../../../components/credentials/CredentialsList.tsx';
import type { Credential } from '../../../types/credential.ts';

const mockCredentials: Credential[] = [
  {
    credential_id: 'cred-1',
    user_id: 'user-1',
    name: 'OPENAI_API_KEY',
    created_at: '2026-01-15T10:00:00Z',
    updated_at: '2026-01-15T10:00:00Z',
  },
  {
    credential_id: 'cred-2',
    user_id: 'user-1',
    name: 'GITHUB_TOKEN',
    created_at: '2026-01-16T10:00:00Z',
    updated_at: '2026-01-16T10:00:00Z',
  },
];

describe('CredentialsList', () => {
  // Set up the delete handler
  beforeEach(() => {
    server.use(
      http.delete('/api/v1/credentials/:id', () =>
        new HttpResponse(null, { status: 204 }),
      ),
    );
  });

  it('renders table headers', () => {
    renderWithProviders(<CredentialsList credentials={mockCredentials} />);
    expect(screen.getByText('Name')).toBeInTheDocument();
    expect(screen.getByText('Value')).toBeInTheDocument();
    expect(screen.getByText('Created')).toBeInTheDocument();
  });

  it('renders credential names', () => {
    renderWithProviders(<CredentialsList credentials={mockCredentials} />);
    expect(screen.getByText('OPENAI_API_KEY')).toBeInTheDocument();
    expect(screen.getByText('GITHUB_TOKEN')).toBeInTheDocument();
  });

  it('renders masked credential values', () => {
    renderWithProviders(<CredentialsList credentials={mockCredentials} />);
    const maskedValues = screen.getAllByText('••••••••');
    expect(maskedValues.length).toBe(2);
  });

  it('renders credential IDs (truncated)', () => {
    renderWithProviders(<CredentialsList credentials={mockCredentials} />);
    // truncateId shows first part of the ID
    expect(screen.getByText('OPENAI_API_KEY').closest('td')).toBeInTheDocument();
  });

  it('renders delete button for each credential', () => {
    renderWithProviders(<CredentialsList credentials={mockCredentials} />);
    const deleteButtons = screen.getAllByTitle('Delete credential');
    expect(deleteButtons).toHaveLength(2);
  });

  it('clicking delete button shows confirmation dialog', async () => {
    const { user } = renderWithProviders(
      <CredentialsList credentials={mockCredentials} />,
    );

    const deleteButtons = screen.getAllByTitle('Delete credential');
    await user.click(deleteButtons[0]);

    await waitFor(() => {
      expect(screen.getByText('Delete credential')).toBeInTheDocument();
      expect(screen.getByText(/Are you sure you want to delete "OPENAI_API_KEY"/)).toBeInTheDocument();
    });
  });

  it('confirmation dialog shows credential name in message', async () => {
    const { user } = renderWithProviders(
      <CredentialsList credentials={mockCredentials} />,
    );

    const deleteButtons = screen.getAllByTitle('Delete credential');
    await user.click(deleteButtons[1]);

    await waitFor(() => {
      expect(screen.getByText(/Are you sure you want to delete "GITHUB_TOKEN"/)).toBeInTheDocument();
    });
  });

  it('confirmation dialog has Cancel and Delete buttons', async () => {
    const { user } = renderWithProviders(
      <CredentialsList credentials={mockCredentials} />,
    );

    const deleteButtons = screen.getAllByTitle('Delete credential');
    await user.click(deleteButtons[0]);

    await waitFor(() => {
      expect(screen.getByText('Delete credential')).toBeInTheDocument();
    });

    // The confirm dialog should have Cancel and Delete buttons
    expect(screen.getByText('Cancel')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Delete' })).toBeInTheDocument();
  });

  it('cancelling delete dialog closes it', async () => {
    const { user } = renderWithProviders(
      <CredentialsList credentials={mockCredentials} />,
    );

    const deleteButtons = screen.getAllByTitle('Delete credential');
    await user.click(deleteButtons[0]);

    await waitFor(() => {
      expect(screen.getByText('Delete credential')).toBeInTheDocument();
    });

    await user.click(screen.getByText('Cancel'));

    await waitFor(() => {
      expect(screen.queryByText(/Are you sure you want to delete/)).not.toBeInTheDocument();
    });
  });

  it('confirming delete calls the delete API', async () => {
    let deletedId: string | null = null;

    server.use(
      http.delete('/api/v1/credentials/:id', ({ params }) => {
        deletedId = params.id as string;
        return new HttpResponse(null, { status: 204 });
      }),
    );

    const { user } = renderWithProviders(
      <CredentialsList credentials={mockCredentials} />,
    );

    const deleteButtons = screen.getAllByTitle('Delete credential');
    await user.click(deleteButtons[0]);

    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'Delete' })).toBeInTheDocument();
    });

    await user.click(screen.getByRole('button', { name: 'Delete' }));

    await waitFor(() => {
      expect(deletedId).toBe('cred-1');
    });
  });

  it('renders empty table with no credentials', () => {
    renderWithProviders(<CredentialsList credentials={[]} />);
    expect(screen.getByText('Name')).toBeInTheDocument();
    // No credential rows
    expect(screen.queryByTitle('Delete credential')).not.toBeInTheDocument();
  });

  it('shows "This action cannot be undone" in delete confirmation', async () => {
    const { user } = renderWithProviders(
      <CredentialsList credentials={mockCredentials} />,
    );

    const deleteButtons = screen.getAllByTitle('Delete credential');
    await user.click(deleteButtons[0]);

    await waitFor(() => {
      expect(screen.getByText(/This action cannot be undone/)).toBeInTheDocument();
    });
  });
});
