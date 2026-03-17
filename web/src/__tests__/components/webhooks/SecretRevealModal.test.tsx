import { describe, it, expect, vi } from 'vitest';
import { renderWithProviders, screen } from '../../../test/render.tsx';
import { SecretRevealModal } from '../../../components/webhooks/SecretRevealModal.tsx';

describe('SecretRevealModal', () => {
  it('renders nothing when closed', () => {
    renderWithProviders(
      <SecretRevealModal open={false} secret="my-secret" onClose={() => {}} />,
    );
    expect(screen.queryByText('Webhook Secret')).not.toBeInTheDocument();
  });

  it('shows warning message and secret when open', () => {
    renderWithProviders(
      <SecretRevealModal open={true} secret="super-secret-123" onClose={() => {}} />,
    );

    expect(screen.getByText('Webhook Secret')).toBeInTheDocument();
    expect(
      screen.getByText("Save your webhook secret -- it won't be shown again."),
    ).toBeInTheDocument();
    expect(screen.getByText('super-secret-123')).toBeInTheDocument();
    expect(screen.getByText('Done')).toBeInTheDocument();
  });

  it('calls onClose when Done is clicked', async () => {
    const onClose = vi.fn();
    const { user } = renderWithProviders(
      <SecretRevealModal open={true} secret="test-secret" onClose={onClose} />,
    );

    await user.click(screen.getByText('Done'));
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('has a copy button', () => {
    renderWithProviders(
      <SecretRevealModal open={true} secret="copy-me" onClose={() => {}} />,
    );
    expect(screen.getByTitle('Copy to clipboard')).toBeInTheDocument();
  });
});
