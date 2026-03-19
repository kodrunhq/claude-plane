import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, it, expect, vi } from 'vitest';
import { RefreshButton } from '../../../components/shared/RefreshButton.tsx';

describe('RefreshButton', () => {
  it('renders a button with "Refresh" text', () => {
    render(<RefreshButton onClick={vi.fn()} />);
    expect(screen.getByRole('button', { name: /refresh/i })).toBeInTheDocument();
  });

  it('calls onClick when clicked', async () => {
    const user = userEvent.setup();
    const handleClick = vi.fn();

    render(<RefreshButton onClick={handleClick} />);
    await user.click(screen.getByRole('button'));

    expect(handleClick).toHaveBeenCalledTimes(1);
  });

  it('is not disabled by default', () => {
    render(<RefreshButton onClick={vi.fn()} />);
    expect(screen.getByRole('button')).not.toBeDisabled();
  });

  describe('loading state', () => {
    it('disables the button when loading is true', () => {
      render(<RefreshButton onClick={vi.fn()} loading />);
      expect(screen.getByRole('button')).toBeDisabled();
    });

    it('does not call onClick when loading and clicked', async () => {
      const user = userEvent.setup();
      const handleClick = vi.fn();

      render(<RefreshButton onClick={handleClick} loading />);
      await user.click(screen.getByRole('button'));

      expect(handleClick).not.toHaveBeenCalled();
    });

    it('applies animate-spin class to icon when loading', () => {
      const { container } = render(<RefreshButton onClick={vi.fn()} loading />);
      const svg = container.querySelector('svg');
      expect(svg).toHaveClass('animate-spin');
    });

    it('does not apply animate-spin class when not loading', () => {
      const { container } = render(<RefreshButton onClick={vi.fn()} />);
      const svg = container.querySelector('svg');
      expect(svg).not.toHaveClass('animate-spin');
    });
  });

  it('applies custom className', () => {
    render(<RefreshButton onClick={vi.fn()} className="ml-4" />);
    const button = screen.getByRole('button');
    expect(button).toHaveClass('ml-4');
  });

  it('has type="button" to prevent form submission', () => {
    render(<RefreshButton onClick={vi.fn()} />);
    expect(screen.getByRole('button')).toHaveAttribute('type', 'button');
  });
});
