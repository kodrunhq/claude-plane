import { describe, it, expect, vi } from 'vitest';
import { http, HttpResponse } from 'msw';
import { server } from '../../test/setup.ts';
import { renderWithProviders, screen, waitFor } from '../../test/render.tsx';
import { AdminPage } from '../../views/AdminPage.tsx';
import type { User } from '../../types/user.ts';

const mockUsers: User[] = [
  {
    user_id: 'user-100',
    email: 'admin@example.com',
    display_name: 'Admin User',
    role: 'admin',
    created_at: '2026-01-10T08:00:00Z',
    updated_at: '2026-01-10T08:00:00Z',
  },
  {
    user_id: 'user-101',
    email: 'dev@example.com',
    display_name: 'Developer',
    role: 'user',
    created_at: '2026-01-12T10:00:00Z',
    updated_at: '2026-01-12T10:00:00Z',
  },
];

function setupUsersHandler(users: User[] = mockUsers) {
  server.use(
    http.get('/api/v1/users', () => HttpResponse.json(users)),
  );
}

describe('AdminPage', () => {
  it('renders the page heading and description', () => {
    setupUsersHandler();
    renderWithProviders(<AdminPage />);

    expect(screen.getByRole('heading', { name: 'Users' })).toBeInTheDocument();
    expect(screen.getByText('Manage user accounts and roles')).toBeInTheDocument();
  });

  it('renders the users list from API data', async () => {
    setupUsersHandler();
    renderWithProviders(<AdminPage />);

    await waitFor(() => {
      expect(screen.getByText('Admin User')).toBeInTheDocument();
      expect(screen.getByText('Developer')).toBeInTheDocument();
    });
  });

  it('shows user emails in the table', async () => {
    setupUsersHandler();
    renderWithProviders(<AdminPage />);

    await waitFor(() => {
      expect(screen.getByText('admin@example.com')).toBeInTheDocument();
      expect(screen.getByText('dev@example.com')).toBeInTheDocument();
    });
  });

  it('shows role badges for users', async () => {
    setupUsersHandler();
    renderWithProviders(<AdminPage />);

    await waitFor(() => {
      expect(screen.getByText('admin')).toBeInTheDocument();
      expect(screen.getByText('user')).toBeInTheDocument();
    });
  });

  it('shows table column headers', async () => {
    setupUsersHandler();
    renderWithProviders(<AdminPage />);

    await waitFor(() => {
      expect(screen.getByText('Name')).toBeInTheDocument();
      expect(screen.getByText('Email')).toBeInTheDocument();
      expect(screen.getByText('Role')).toBeInTheDocument();
      expect(screen.getByText('Created')).toBeInTheDocument();
    });
  });

  it('shows loading skeleton while fetching', () => {
    server.use(
      http.get('/api/v1/users', async () => {
        await new Promise((r) => setTimeout(r, 5000));
        return HttpResponse.json(mockUsers);
      }),
    );

    renderWithProviders(<AdminPage />);

    expect(screen.queryByText('No users yet')).not.toBeInTheDocument();
    expect(screen.queryByText('Admin User')).not.toBeInTheDocument();
  });

  it('shows empty state when no users exist', async () => {
    setupUsersHandler([]);
    renderWithProviders(<AdminPage />);

    await waitFor(() => {
      expect(screen.getByText('No users yet')).toBeInTheDocument();
    });

    expect(screen.getByText('Create user accounts to grant access to claude-plane.')).toBeInTheDocument();
  });

  it('shows error state with retry button when API fails', async () => {
    server.use(
      http.get('/api/v1/users', () =>
        HttpResponse.json({ error: 'Server error' }, { status: 500 }),
      ),
    );

    renderWithProviders(<AdminPage />);

    await waitFor(() => {
      expect(screen.getByText('Retry')).toBeInTheDocument();
    });
  });

  it('renders New User button in header', () => {
    setupUsersHandler();
    renderWithProviders(<AdminPage />);

    expect(screen.getByText('New User')).toBeInTheDocument();
  });

  it('opens create user modal when New User button is clicked', async () => {
    setupUsersHandler();
    const { user } = renderWithProviders(<AdminPage />);

    await waitFor(() => {
      expect(screen.getByText('Admin User')).toBeInTheDocument();
    });

    // Click the header New User button
    const buttons = screen.getAllByText('New User');
    await user.click(buttons[0]);

    await waitFor(() => {
      // CreateUserModal renders a heading "New User" inside the modal
      // and input fields for Email, Password, Display Name, Role
      expect(screen.getByPlaceholderText('user@example.com')).toBeInTheDocument();
      expect(screen.getByPlaceholderText('Min 8 characters')).toBeInTheDocument();
    });
  });

  it('opens create user modal from empty state action button', async () => {
    setupUsersHandler([]);
    const { user } = renderWithProviders(<AdminPage />);

    await waitFor(() => {
      expect(screen.getByText('No users yet')).toBeInTheDocument();
    });

    const newUserButtons = screen.getAllByText('New User');
    await user.click(newUserButtons[newUserButtons.length - 1]);

    await waitFor(() => {
      expect(screen.getByPlaceholderText('user@example.com')).toBeInTheDocument();
    });
  });

  it('closes create user modal when Cancel is clicked', async () => {
    setupUsersHandler();
    const { user } = renderWithProviders(<AdminPage />);

    await waitFor(() => {
      expect(screen.getByText('Admin User')).toBeInTheDocument();
    });

    // Open modal
    const buttons = screen.getAllByText('New User');
    await user.click(buttons[0]);

    await waitFor(() => {
      expect(screen.getByPlaceholderText('user@example.com')).toBeInTheDocument();
    });

    // Click Cancel
    await user.click(screen.getByText('Cancel'));

    await waitFor(() => {
      expect(screen.queryByPlaceholderText('user@example.com')).not.toBeInTheDocument();
    });
  });

  it('submits create user form with correct data', async () => {
    let capturedBody: Record<string, unknown> | null = null;
    setupUsersHandler();
    server.use(
      http.post('/api/v1/users', async ({ request }) => {
        capturedBody = await request.json() as Record<string, unknown>;
        return HttpResponse.json(
          {
            user_id: 'user-new',
            email: capturedBody.email,
            display_name: capturedBody.display_name,
            role: capturedBody.role,
            created_at: '2026-01-15T12:00:00Z',
            updated_at: '2026-01-15T12:00:00Z',
          },
          { status: 201 },
        );
      }),
    );

    const { user } = renderWithProviders(<AdminPage />);

    await waitFor(() => {
      expect(screen.getByText('Admin User')).toBeInTheDocument();
    });

    // Open modal
    const buttons = screen.getAllByText('New User');
    await user.click(buttons[0]);

    await waitFor(() => {
      expect(screen.getByPlaceholderText('user@example.com')).toBeInTheDocument();
    });

    // Fill form
    await user.type(screen.getByPlaceholderText('user@example.com'), 'new@example.com');
    await user.type(screen.getByPlaceholderText('Min 8 characters'), 'password123');
    await user.type(screen.getByPlaceholderText('Optional'), 'New Guy');

    // Submit
    await user.click(screen.getByText('Create User'));

    await waitFor(() => {
      expect(capturedBody).not.toBeNull();
    });

    expect(capturedBody!.email).toBe('new@example.com');
    expect(capturedBody!.password).toBe('password123');
    expect(capturedBody!.display_name).toBe('New Guy');
    expect(capturedBody!.role).toBe('user');
  });

  it('renders edit and delete action buttons for each user', async () => {
    setupUsersHandler();
    renderWithProviders(<AdminPage />);

    await waitFor(() => {
      expect(screen.getByText('Admin User')).toBeInTheDocument();
    });

    // Each user row has Edit user, Delete user, and Reset password buttons (via title attributes)
    const editButtons = screen.getAllByTitle('Edit user');
    expect(editButtons).toHaveLength(mockUsers.length);

    const deleteButtons = screen.getAllByTitle('Delete user');
    expect(deleteButtons).toHaveLength(mockUsers.length);

    const resetButtons = screen.getAllByTitle('Reset password');
    expect(resetButtons).toHaveLength(mockUsers.length);
  });

  it('shows delete confirmation dialog when delete button is clicked', async () => {
    setupUsersHandler();
    const { user } = renderWithProviders(<AdminPage />);

    await waitFor(() => {
      expect(screen.getByText('Admin User')).toBeInTheDocument();
    });

    const deleteButtons = screen.getAllByTitle('Delete user');
    await user.click(deleteButtons[0]);

    await waitFor(() => {
      expect(screen.getByText(/Are you sure you want to delete/)).toBeInTheDocument();
    });

    // The confirm dialog message includes the email
    expect(screen.getByText(/Are you sure you want to delete "admin@example.com"/)).toBeInTheDocument();
  });

  it('calls delete API when confirm delete is clicked', async () => {
    let deleteCalled = false;
    setupUsersHandler();
    server.use(
      http.delete('/api/v1/users/:id', () => {
        deleteCalled = true;
        return new HttpResponse(null, { status: 204 });
      }),
    );

    const { user } = renderWithProviders(<AdminPage />);

    await waitFor(() => {
      expect(screen.getByText('Admin User')).toBeInTheDocument();
    });

    // Click delete on first user
    const deleteButtons = screen.getAllByTitle('Delete user');
    await user.click(deleteButtons[0]);

    await waitFor(() => {
      expect(screen.getByText(/Are you sure/)).toBeInTheDocument();
    });

    // Confirm deletion
    await user.click(screen.getByText('Delete'));

    await waitFor(() => {
      expect(deleteCalled).toBe(true);
    });
  });
});
