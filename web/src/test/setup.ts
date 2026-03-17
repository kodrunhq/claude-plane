// Shared test setup — imported via vite.config.ts setupFiles

import '@testing-library/jest-dom';
import { setupServer } from 'msw/node';
import { handlers } from './handlers.ts';
import { resetFactories } from './factories.ts';

export const server = setupServer(...handlers);

beforeAll(() => server.listen({ onUnhandledRequest: 'warn' }));
afterEach(() => {
  server.resetHandlers();
  resetFactories();
});
afterAll(() => server.close());
