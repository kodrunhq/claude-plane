import { describe, it, expect, vi } from 'vitest';
import { renderWithProviders, screen } from '../../../test/render.tsx';
import { ConfirmDialog } from '../../../components/shared/ConfirmDialog.tsx';

describe('ConfirmDialog', () => {
  const defaultProps = {
    open: true,
    title: 'Delete Item',
    message: 'Are you sure you want to delete this item?',
    onConfirm: vi.fn(),
    onCancel: vi.fn(),
  };

  it('is not visible when open=false', () => {
    renderWithProviders(
      <ConfirmDialog {...defaultProps} open={false} />,
    );

    expect(screen.queryByText('Delete Item')).not.toBeInTheDocument();
    expect(screen.queryByText('Are you sure you want to delete this item?')).not.toBeInTheDocument();
  });

  it('is visible when open=true', () => {
    renderWithProviders(
      <ConfirmDialog {...defaultProps} />,
    );

    expect(screen.getByText('Delete Item')).toBeInTheDocument();
    expect(screen.getByText('Are you sure you want to delete this item?')).toBeInTheDocument();
  });

  it('shows title and message', () => {
    renderWithProviders(
      <ConfirmDialog
        {...defaultProps}
        title="Confirm Action"
        message="This cannot be undone."
      />,
    );

    expect(screen.getByText('Confirm Action')).toBeInTheDocument();
    expect(screen.getByText('This cannot be undone.')).toBeInTheDocument();
  });

  it('confirm button calls onConfirm', async () => {
    const onConfirm = vi.fn();
    const { user } = renderWithProviders(
      <ConfirmDialog {...defaultProps} onConfirm={onConfirm} />,
    );

    await user.click(screen.getByText('Confirm'));

    expect(onConfirm).toHaveBeenCalledTimes(1);
  });

  it('cancel button calls onCancel', async () => {
    const onCancel = vi.fn();
    const { user } = renderWithProviders(
      <ConfirmDialog {...defaultProps} onCancel={onCancel} />,
    );

    await user.click(screen.getByText('Cancel'));

    expect(onCancel).toHaveBeenCalledTimes(1);
  });

  it('clicking backdrop calls onCancel', async () => {
    const onCancel = vi.fn();
    const { user } = renderWithProviders(
      <ConfirmDialog {...defaultProps} onCancel={onCancel} />,
    );

    // The backdrop is the div with bg-black/60 class
    const backdrop = screen.getByText('Delete Item').closest('.fixed')!.querySelector('.bg-black\\/60')!;
    await user.click(backdrop);

    expect(onCancel).toHaveBeenCalledTimes(1);
  });

  it('Escape key calls onCancel', async () => {
    const onCancel = vi.fn();
    const { user } = renderWithProviders(
      <ConfirmDialog {...defaultProps} onCancel={onCancel} />,
    );

    await user.keyboard('{Escape}');

    expect(onCancel).toHaveBeenCalledTimes(1);
  });

  it('danger variant has red styling on confirm button', () => {
    renderWithProviders(
      <ConfirmDialog {...defaultProps} variant="danger" confirmLabel="Delete" />,
    );

    const confirmButton = screen.getByText('Delete');
    expect(confirmButton.className).toContain('bg-status-error');
  });

  it('default variant has accent styling on confirm button', () => {
    renderWithProviders(
      <ConfirmDialog {...defaultProps} />,
    );

    const confirmButton = screen.getByText('Confirm');
    expect(confirmButton.className).toContain('bg-accent-primary');
  });

  it('confirm button shows confirmLabel text', () => {
    renderWithProviders(
      <ConfirmDialog {...defaultProps} confirmLabel="Yes, Delete" />,
    );

    expect(screen.getByText('Yes, Delete')).toBeInTheDocument();
    expect(screen.queryByText('Confirm')).not.toBeInTheDocument();
  });

  it('defaults confirmLabel to "Confirm" when not provided', () => {
    renderWithProviders(
      <ConfirmDialog
        open={true}
        title="Test"
        message="Test message"
        onConfirm={vi.fn()}
        onCancel={vi.fn()}
      />,
    );

    expect(screen.getByText('Confirm')).toBeInTheDocument();
  });
});
