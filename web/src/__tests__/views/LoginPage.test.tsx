import { describe, it, expect } from 'vitest';
import { http, HttpResponse } from 'msw';
import { server } from '../../test/setup.ts';
import { renderWithProviders, screen, waitFor } from '../../test/render.tsx';
import { LoginPage } from '../../views/LoginPage.tsx';
import { useAuthStore } from '../../stores/auth.ts';

describe('LoginPage', () => {
  it('renders the login form with heading, email and password inputs, and sign-in button', () => {
    renderWithProviders(<LoginPage />);

    expect(screen.getByRole('heading', { name: /claude-plane/i })).toBeInTheDocument();
    expect(screen.getByLabelText('Email')).toBeInTheDocument();
    expect(screen.getByLabelText('Password')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /sign in/i })).toBeInTheDocument();
  });

  it('renders subtitle text', () => {
    renderWithProviders(<LoginPage />);

    expect(screen.getByText('Control Plane for Claude CLI')).toBeInTheDocument();
  });

  it('email input has correct type and placeholder', () => {
    renderWithProviders(<LoginPage />);

    const emailInput = screen.getByLabelText('Email');
    expect(emailInput).toHaveAttribute('type', 'email');
    expect(emailInput).toHaveAttribute('placeholder', 'admin@localhost');
  });

  it('password input has correct type', () => {
    renderWithProviders(<LoginPage />);

    const passwordInput = screen.getByLabelText('Password');
    expect(passwordInput).toHaveAttribute('type', 'password');
  });

  it('allows typing into email and password fields', async () => {
    const { user } = renderWithProviders(<LoginPage />);

    const emailInput = screen.getByLabelText('Email');
    const passwordInput = screen.getByLabelText('Password');

    await user.type(emailInput, 'test@example.com');
    await user.type(passwordInput, 'secret123');

    expect(emailInput).toHaveValue('test@example.com');
    expect(passwordInput).toHaveValue('secret123');
  });

  it('calls login API on form submit', async () => {
    server.use(
      http.post('/api/v1/auth/login', async () =>
        HttpResponse.json({
          user_id: 'u-1',
          email: 'test@example.com',
          display_name: 'Test',
          role: 'admin',
        }),
      ),
    );

    const { user } = renderWithProviders(<LoginPage />);

    await user.type(screen.getByLabelText('Email'), 'test@example.com');
    await user.type(screen.getByLabelText('Password'), 'secret123');
    await user.click(screen.getByRole('button', { name: /sign in/i }));

    await waitFor(() => {
      const state = useAuthStore.getState();
      expect(state.authenticated).toBe(true);
      expect(state.user?.email).toBe('test@example.com');
    });
  });

  it('displays error message on login failure', async () => {
    server.use(
      http.post('/api/v1/auth/login', () =>
        HttpResponse.json({ error: 'Invalid credentials' }, { status: 401 }),
      ),
    );

    const { user } = renderWithProviders(<LoginPage />);

    await user.type(screen.getByLabelText('Email'), 'bad@example.com');
    await user.type(screen.getByLabelText('Password'), 'wrong');
    await user.click(screen.getByRole('button', { name: /sign in/i }));

    await waitFor(() => {
      expect(screen.getByText('Invalid credentials')).toBeInTheDocument();
    });
  });

  it('shows "Signing in..." text while submitting', async () => {
    // Use a delayed response to keep the submitting state visible
    server.use(
      http.post('/api/v1/auth/login', async () => {
        await new Promise((resolve) => setTimeout(resolve, 200));
        return HttpResponse.json({
          user_id: 'u-1',
          email: 'test@example.com',
          display_name: 'Test',
          role: 'admin',
        });
      }),
    );

    const { user } = renderWithProviders(<LoginPage />);

    await user.type(screen.getByLabelText('Email'), 'test@example.com');
    await user.type(screen.getByLabelText('Password'), 'secret123');
    await user.click(screen.getByRole('button', { name: /sign in/i }));

    expect(screen.getByRole('button', { name: /signing in/i })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /signing in/i })).toBeDisabled();

    // Wait for the request to complete
    await waitFor(() => {
      expect(screen.getByRole('button', { name: /sign in/i })).toBeInTheDocument();
    });
  });

  it('shows generic error message when response has no error field', async () => {
    server.use(
      http.post('/api/v1/auth/login', () =>
        HttpResponse.json({}, { status: 500 }),
      ),
    );

    const { user } = renderWithProviders(<LoginPage />);

    await user.type(screen.getByLabelText('Email'), 'test@example.com');
    await user.type(screen.getByLabelText('Password'), 'secret123');
    await user.click(screen.getByRole('button', { name: /sign in/i }));

    await waitFor(() => {
      expect(screen.getByText(/login failed/i)).toBeInTheDocument();
    });
  });
});
