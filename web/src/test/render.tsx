// Shared render helper — wraps components in providers needed for most tests

import { render, type RenderOptions, screen, waitFor, within } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router';
import userEvent from '@testing-library/user-event';
import type { ReactElement } from 'react';

interface RenderWithProvidersOptions extends Omit<RenderOptions, 'wrapper'> {
  /** Initial route entries for MemoryRouter */
  routes?: string[];
  /** Provide your own QueryClient (e.g., with custom defaults) */
  queryClient?: QueryClient;
}

/**
 * Renders a component wrapped in QueryClientProvider + MemoryRouter.
 *
 * Returns the standard render result plus a pre-configured `user` from
 * @testing-library/user-event and the QueryClient instance.
 */
export function renderWithProviders(
  ui: ReactElement,
  options: RenderWithProvidersOptions = {},
) {
  const { routes = ['/'], queryClient: externalClient, ...renderOptions } = options;

  const queryClient =
    externalClient ??
    new QueryClient({
      defaultOptions: {
        queries: { retry: false },
      },
    });

  function Wrapper({ children }: { children: React.ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>
        <MemoryRouter initialEntries={routes}>{children}</MemoryRouter>
      </QueryClientProvider>
    );
  }

  const result = render(ui, { wrapper: Wrapper, ...renderOptions });
  const user = userEvent.setup();

  return { ...result, user, queryClient };
}

// Re-export commonly used testing utilities for convenience
export { screen, waitFor, within, userEvent };
