import type React from 'react';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { ErrorBoundary } from '../../../components/shared/ErrorBoundary.tsx';

// A component that throws on render — return type annotation satisfies JSX element constraint
function ThrowingComponent({ message }: { message: string }): React.ReactNode {
  throw new Error(message);
}

// A component that renders normally
function GoodComponent() {
  return <div>All is well</div>;
}

describe('ErrorBoundary', () => {
  // Suppress React's error boundary console.error noise in tests
  beforeEach(() => {
    vi.spyOn(console, 'error').mockImplementation(() => {});
  });

  it('renders children when no error occurs', () => {
    render(
      <ErrorBoundary>
        <GoodComponent />
      </ErrorBoundary>,
    );
    expect(screen.getByText('All is well')).toBeInTheDocument();
  });

  it('renders fallback UI when a child throws', () => {
    render(
      <ErrorBoundary>
        <ThrowingComponent message="Test error" />
      </ErrorBoundary>,
    );

    expect(screen.getByText('Something went wrong')).toBeInTheDocument();
    expect(
      screen.getByText('An unexpected error occurred. You can try going back to the home page.'),
    ).toBeInTheDocument();
  });

  it('renders the "Go Home" button in fallback UI', () => {
    render(
      <ErrorBoundary>
        <ThrowingComponent message="Crash" />
      </ErrorBoundary>,
    );

    expect(screen.getByRole('button', { name: 'Go Home' })).toBeInTheDocument();
  });

  it('navigates to "/" when "Go Home" is clicked', async () => {
    const user = userEvent.setup();

    // Mock window.location.href
    const locationSpy = vi.spyOn(window, 'location', 'get').mockReturnValue({
      ...window.location,
      href: '',
    } as Location);

    // Override location.href via Object.defineProperty to avoid strict type issues
    const originalLocation = window.location;
    const hrefDescriptor = Object.getOwnPropertyDescriptor(window, 'location');
    Object.defineProperty(window, 'location', {
      writable: true,
      value: { ...originalLocation, href: '' },
    });

    render(
      <ErrorBoundary>
        <ThrowingComponent message="Crash" />
      </ErrorBoundary>,
    );

    await user.click(screen.getByRole('button', { name: 'Go Home' }));
    expect(window.location.href).toBe('/');

    // Restore
    if (hrefDescriptor) {
      Object.defineProperty(window, 'location', hrefDescriptor);
    }
    locationSpy.mockRestore();
  });

  it('shows error message in DEV mode', () => {
    // import.meta.env.DEV is true in vitest by default
    render(
      <ErrorBoundary>
        <ThrowingComponent message="Detailed error info" />
      </ErrorBoundary>,
    );

    expect(screen.getByText('Detailed error info')).toBeInTheDocument();
  });

  it('logs the error via console.error', () => {
    const consoleSpy = vi.spyOn(console, 'error').mockImplementation(() => {});

    render(
      <ErrorBoundary>
        <ThrowingComponent message="Logged error" />
      </ErrorBoundary>,
    );

    expect(consoleSpy).toHaveBeenCalledWith(
      '[ErrorBoundary] Uncaught error:',
      expect.any(Error),
      expect.anything(),
    );
  });

  it('displays the error message in a pre element', () => {
    const { container } = render(
      <ErrorBoundary>
        <ThrowingComponent message="Pre-formatted error" />
      </ErrorBoundary>,
    );

    const pre = container.querySelector('pre');
    expect(pre).toBeInTheDocument();
    expect(pre).toHaveTextContent('Pre-formatted error');
  });

  it('does not show fallback UI when no error', () => {
    render(
      <ErrorBoundary>
        <GoodComponent />
      </ErrorBoundary>,
    );

    expect(screen.queryByText('Something went wrong')).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Go Home' })).not.toBeInTheDocument();
  });
});
