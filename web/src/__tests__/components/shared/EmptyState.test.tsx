import { render, screen } from '@testing-library/react';
import { describe, it, expect } from 'vitest';
import { EmptyState } from '../../../components/shared/EmptyState.tsx';

describe('EmptyState', () => {
  it('renders the title', () => {
    render(<EmptyState title="No items found" />);
    expect(screen.getByText('No items found')).toBeInTheDocument();
  });

  it('renders the title as an h3 element', () => {
    render(<EmptyState title="No items found" />);
    const heading = screen.getByRole('heading', { level: 3 });
    expect(heading).toHaveTextContent('No items found');
  });

  describe('optional description', () => {
    it('renders description when provided', () => {
      render(<EmptyState title="Empty" description="Try creating a new item." />);
      expect(screen.getByText('Try creating a new item.')).toBeInTheDocument();
    });

    it('does not render description paragraph when not provided', () => {
      const { container } = render(<EmptyState title="Empty" />);
      const paragraphs = container.querySelectorAll('p');
      expect(paragraphs).toHaveLength(0);
    });
  });

  describe('optional icon', () => {
    it('renders icon when provided', () => {
      render(
        <EmptyState
          title="Empty"
          icon={<span data-testid="custom-icon">Icon</span>}
        />,
      );
      expect(screen.getByTestId('custom-icon')).toBeInTheDocument();
    });

    it('does not render icon container when icon is not provided', () => {
      const { container } = render(<EmptyState title="Empty" />);
      // The icon wrapper div has opacity-50 class
      const iconWrapper = container.querySelector('.opacity-50');
      expect(iconWrapper).not.toBeInTheDocument();
    });
  });

  describe('optional action', () => {
    it('renders action button when provided', () => {
      render(
        <EmptyState
          title="Empty"
          action={<button type="button">Create Item</button>}
        />,
      );
      expect(screen.getByRole('button', { name: 'Create Item' })).toBeInTheDocument();
    });

    it('does not render action container when action is not provided', () => {
      const { container } = render(<EmptyState title="Empty" />);
      // Should only have the main wrapper div and the h3
      const buttons = container.querySelectorAll('button');
      expect(buttons).toHaveLength(0);
    });
  });

  it('renders all optional props together', () => {
    render(
      <EmptyState
        title="No sessions"
        description="Start a new session to get going."
        icon={<span data-testid="icon">SessionIcon</span>}
        action={<button type="button">New Session</button>}
      />,
    );

    expect(screen.getByText('No sessions')).toBeInTheDocument();
    expect(screen.getByText('Start a new session to get going.')).toBeInTheDocument();
    expect(screen.getByTestId('icon')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'New Session' })).toBeInTheDocument();
  });
});
