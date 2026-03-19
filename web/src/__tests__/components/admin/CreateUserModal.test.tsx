import { describe, it, expect, vi } from 'vitest';
import { renderWithProviders, screen, waitFor } from '../../../test/render.tsx';
import { CreateUserModal } from '../../../components/admin/CreateUserModal.tsx';
import type { CreateUserParams } from '../../../types/user.ts';

describe('CreateUserModal', () => {
  const defaultProps = {
    open: true,
    onClose: vi.fn(),
    onSubmit: vi.fn<(params: CreateUserParams) => Promise<void>>().mockResolvedValue(undefined),
    submitting: false,
  };

  function renderModal(overrides?: Partial<typeof defaultProps>) {
    const props = { ...defaultProps, ...overrides };
    return renderWithProviders(<CreateUserModal {...props} />);
  }

  it('renders nothing when open is false', () => {
    renderModal({ open: false });
    expect(screen.queryByText('New User')).not.toBeInTheDocument();
  });

  it('renders modal title when open', () => {
    renderModal();
    expect(screen.getByText('New User')).toBeInTheDocument();
  });

  it('renders email input field', () => {
    renderModal();
    expect(screen.getByPlaceholderText('user@example.com')).toBeInTheDocument();
  });

  it('renders password input field', () => {
    renderModal();
    expect(screen.getByPlaceholderText('Min 8 characters')).toBeInTheDocument();
  });

  it('renders display name input field', () => {
    renderModal();
    expect(screen.getByPlaceholderText('Optional')).toBeInTheDocument();
  });

  it('renders role select defaulting to "user"', () => {
    renderModal();
    expect(screen.getByDisplayValue('user')).toBeInTheDocument();
  });

  it('renders Cancel and Create User buttons', () => {
    renderModal();
    expect(screen.getByText('Cancel')).toBeInTheDocument();
    expect(screen.getByText('Create User')).toBeInTheDocument();
  });

  it('email field accepts input', async () => {
    const { user } = renderModal();
    const emailInput = screen.getByPlaceholderText('user@example.com');
    await user.type(emailInput, 'test@example.com');
    expect(emailInput).toHaveValue('test@example.com');
  });

  it('password field accepts input', async () => {
    const { user } = renderModal();
    const passwordInput = screen.getByPlaceholderText('Min 8 characters');
    await user.type(passwordInput, 'securepass');
    expect(passwordInput).toHaveValue('securepass');
  });

  it('display name field accepts input', async () => {
    const { user } = renderModal();
    const nameInput = screen.getByPlaceholderText('Optional');
    await user.type(nameInput, 'John Doe');
    expect(nameInput).toHaveValue('John Doe');
  });

  it('role select can be changed to admin', async () => {
    const { user } = renderModal();
    const select = screen.getByDisplayValue('user');
    await user.selectOptions(select, 'admin');
    expect(select).toHaveValue('admin');
  });

  it('email input has type="email"', () => {
    renderModal();
    const emailInput = screen.getByPlaceholderText('user@example.com');
    expect(emailInput).toHaveAttribute('type', 'email');
  });

  it('password input has type="password"', () => {
    renderModal();
    const passwordInput = screen.getByPlaceholderText('Min 8 characters');
    expect(passwordInput).toHaveAttribute('type', 'password');
  });

  it('email input is required', () => {
    renderModal();
    const emailInput = screen.getByPlaceholderText('user@example.com');
    expect(emailInput).toBeRequired();
  });

  it('password input is required with minLength=8', () => {
    renderModal();
    const passwordInput = screen.getByPlaceholderText('Min 8 characters');
    expect(passwordInput).toBeRequired();
    expect(passwordInput).toHaveAttribute('minlength', '8');
  });

  it('Cancel button calls onClose', async () => {
    const onClose = vi.fn();
    const { user } = renderModal({ onClose });

    await user.click(screen.getByText('Cancel'));
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('X close button calls onClose', async () => {
    const onClose = vi.fn();
    const { user } = renderModal({ onClose });

    const closeButton = screen.getByLabelText('Close');
    await user.click(closeButton);
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('clicking backdrop calls onClose', async () => {
    const onClose = vi.fn();
    const { user } = renderModal({ onClose });

    const backdrop = screen.getByText('New User').closest('.fixed')!.querySelector('.absolute');
    expect(backdrop).toBeTruthy();
    await user.click(backdrop!);
    expect(onClose).toHaveBeenCalled();
  });

  it('submit calls onSubmit with form data', async () => {
    const onSubmit = vi.fn<(params: CreateUserParams) => Promise<void>>().mockResolvedValue(undefined);
    const { user } = renderModal({ onSubmit });

    await user.type(screen.getByPlaceholderText('user@example.com'), 'test@example.com');
    await user.type(screen.getByPlaceholderText('Min 8 characters'), 'password123');
    await user.type(screen.getByPlaceholderText('Optional'), 'Test User');
    await user.selectOptions(screen.getByDisplayValue('user'), 'admin');

    await user.click(screen.getByText('Create User'));

    await waitFor(() => {
      expect(onSubmit).toHaveBeenCalledWith({
        email: 'test@example.com',
        password: 'password123',
        display_name: 'Test User',
        role: 'admin',
      });
    });
  });

  it('form resets after successful submit', async () => {
    const onSubmit = vi.fn<(params: CreateUserParams) => Promise<void>>().mockResolvedValue(undefined);
    const { user } = renderModal({ onSubmit });

    await user.type(screen.getByPlaceholderText('user@example.com'), 'test@example.com');
    await user.type(screen.getByPlaceholderText('Min 8 characters'), 'password123');
    await user.click(screen.getByText('Create User'));

    await waitFor(() => {
      expect(onSubmit).toHaveBeenCalled();
    });

    // After successful submit, fields should be reset
    await waitFor(() => {
      expect(screen.getByPlaceholderText('user@example.com')).toHaveValue('');
    });
  });

  it('shows "Creating..." when submitting is true', () => {
    renderModal({ submitting: true });
    expect(screen.getByText('Creating...')).toBeInTheDocument();
    expect(screen.queryByText('Create User')).not.toBeInTheDocument();
  });

  it('submit button is disabled when submitting', () => {
    renderModal({ submitting: true });
    expect(screen.getByText('Creating...')).toBeDisabled();
  });

  it('shows "Create User" when not submitting', () => {
    renderModal({ submitting: false });
    expect(screen.getByText('Create User')).toBeInTheDocument();
  });

  it('form preserves state on submit failure', async () => {
    const onSubmit = vi.fn<(params: CreateUserParams) => Promise<void>>().mockRejectedValue(new Error('fail'));
    const { user } = renderModal({ onSubmit });

    await user.type(screen.getByPlaceholderText('user@example.com'), 'test@example.com');
    await user.type(screen.getByPlaceholderText('Min 8 characters'), 'password123');
    await user.click(screen.getByText('Create User'));

    await waitFor(() => {
      expect(onSubmit).toHaveBeenCalled();
    });

    // Form should preserve values on failure
    expect(screen.getByPlaceholderText('user@example.com')).toHaveValue('test@example.com');
    expect(screen.getByPlaceholderText('Min 8 characters')).toHaveValue('password123');
  });

  it('has user and admin role options', () => {
    renderModal();
    const select = screen.getByDisplayValue('user');
    const options = select.querySelectorAll('option');
    const values = Array.from(options).map((o) => o.value);
    expect(values).toContain('user');
    expect(values).toContain('admin');
  });

  it('shows required asterisk for Email and Password labels', () => {
    renderModal();
    // The "*" is rendered via <span className="text-status-error">*</span>
    const labels = screen.getAllByText('*');
    expect(labels.length).toBe(2);
  });
});
