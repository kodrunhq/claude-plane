import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { describe, it, expect } from 'vitest';
import { Breadcrumb } from '../../../components/shared/Breadcrumb.tsx';

function renderBreadcrumb(items: { label: string; to?: string }[]) {
  return render(
    <MemoryRouter>
      <Breadcrumb items={items} />
    </MemoryRouter>,
  );
}

describe('Breadcrumb', () => {
  it('renders a nav with aria-label', () => {
    renderBreadcrumb([{ label: 'Home' }]);
    expect(screen.getByRole('navigation', { name: 'Breadcrumb' })).toBeInTheDocument();
  });

  it('renders first item as link when it has "to"', () => {
    renderBreadcrumb([
      { label: 'Runs', to: '/runs' },
      { label: 'Run abc12345' },
    ]);
    const link = screen.getByRole('link', { name: 'Runs' });
    expect(link).toHaveAttribute('href', '/runs');
  });

  it('renders last item as plain text (not a link)', () => {
    renderBreadcrumb([
      { label: 'Runs', to: '/runs' },
      { label: 'Run abc12345' },
    ]);
    expect(screen.getByText('Run abc12345')).toBeInTheDocument();
    // Should not be a link
    expect(screen.queryByRole('link', { name: 'Run abc12345' })).not.toBeInTheDocument();
  });

  it('sets aria-current="page" on last item', () => {
    renderBreadcrumb([
      { label: 'Jobs', to: '/jobs' },
      { label: 'deploy-prod' },
    ]);
    const lastItem = screen.getByText('deploy-prod');
    expect(lastItem).toHaveAttribute('aria-current', 'page');
  });

  it('renders single item as current page (not a link)', () => {
    renderBreadcrumb([{ label: 'Dashboard', to: '/' }]);
    expect(screen.getByText('Dashboard')).toBeInTheDocument();
    expect(screen.queryByRole('link')).not.toBeInTheDocument();
    expect(screen.getByText('Dashboard')).toHaveAttribute('aria-current', 'page');
  });
});
