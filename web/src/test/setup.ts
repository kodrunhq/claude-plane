// Shared test setup — imported via vite.config.ts setupFiles

import '@testing-library/jest-dom';
import { setupServer } from 'msw/node';
import { handlers } from './handlers.ts';

export const server = setupServer(...handlers);

beforeAll(() => server.listen({ onUnhandledRequest: 'error' }));
afterEach(() => {
  server.resetHandlers();
});
afterAll(() => server.close());
