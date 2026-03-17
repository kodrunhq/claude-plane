import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { NotFoundPage } from '../../views/NotFoundPage.tsx';

describe('NotFoundPage', () => {
  it('renders "Page not found" text', () => {
    render(
      <MemoryRouter>
        <NotFoundPage />
      </MemoryRouter>,
    );
    expect(screen.getByText('Page not found')).toBeDefined();
  });

  it('has a link to the Command Center', () => {
    render(
      <MemoryRouter>
        <NotFoundPage />
      </MemoryRouter>,
    );
    const link = screen.getByRole('link', { name: /go to command center/i });
    expect(link).toBeDefined();
    expect(link.getAttribute('href')).toBe('/');
  });
});
