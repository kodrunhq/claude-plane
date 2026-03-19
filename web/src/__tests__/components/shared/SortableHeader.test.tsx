import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, it, expect, vi } from 'vitest';
import { SortableHeader } from '../../../components/shared/SortableHeader.tsx';

function renderInTable(ui: React.ReactElement) {
  return render(
    <table>
      <thead>
        <tr>{ui}</tr>
      </thead>
    </table>,
  );
}

describe('SortableHeader', () => {
  it('renders the label text', () => {
    renderInTable(
      <SortableHeader
        label="Name"
        sortKey="name"
        currentSort={null}
        currentDirection="asc"
        onSort={vi.fn()}
      />,
    );
    expect(screen.getByText('Name')).toBeInTheDocument();
  });

  it('renders as a <th> element', () => {
    renderInTable(
      <SortableHeader
        label="Name"
        sortKey="name"
        currentSort={null}
        currentDirection="asc"
        onSort={vi.fn()}
      />,
    );
    expect(screen.getByRole('columnheader')).toBeInTheDocument();
  });

  it('calls onSort with the sortKey when clicked', async () => {
    const user = userEvent.setup();
    const handleSort = vi.fn();

    renderInTable(
      <SortableHeader
        label="Name"
        sortKey="name"
        currentSort={null}
        currentDirection="asc"
        onSort={handleSort}
      />,
    );

    await user.click(screen.getByRole('button'));
    expect(handleSort).toHaveBeenCalledTimes(1);
    expect(handleSort).toHaveBeenCalledWith('name');
  });

  describe('inactive sort state', () => {
    it('does not set aria-sort when not active', () => {
      renderInTable(
        <SortableHeader
          label="Name"
          sortKey="name"
          currentSort="status"
          currentDirection="asc"
          onSort={vi.fn()}
        />,
      );
      const th = screen.getByRole('columnheader');
      expect(th).not.toHaveAttribute('aria-sort');
    });

    it('shows the neutral ChevronsUpDown icon when inactive', () => {
      const { container } = renderInTable(
        <SortableHeader
          label="Name"
          sortKey="name"
          currentSort="status"
          currentDirection="asc"
          onSort={vi.fn()}
        />,
      );
      // ChevronsUpDown icon has opacity-30 class
      const svg = container.querySelector('svg');
      expect(svg).toHaveClass('opacity-30');
    });
  });

  describe('active sort ascending', () => {
    it('sets aria-sort to ascending', () => {
      renderInTable(
        <SortableHeader
          label="Name"
          sortKey="name"
          currentSort="name"
          currentDirection="asc"
          onSort={vi.fn()}
        />,
      );
      const th = screen.getByRole('columnheader');
      expect(th).toHaveAttribute('aria-sort', 'ascending');
    });

    it('shows the ChevronUp icon', () => {
      const { container } = renderInTable(
        <SortableHeader
          label="Name"
          sortKey="name"
          currentSort="name"
          currentDirection="asc"
          onSort={vi.fn()}
        />,
      );
      const svg = container.querySelector('svg');
      // ChevronUp should not have opacity-30 (that's the neutral icon)
      expect(svg).not.toHaveClass('opacity-30');
    });
  });

  describe('active sort descending', () => {
    it('sets aria-sort to descending', () => {
      renderInTable(
        <SortableHeader
          label="Name"
          sortKey="name"
          currentSort="name"
          currentDirection="desc"
          onSort={vi.fn()}
        />,
      );
      const th = screen.getByRole('columnheader');
      expect(th).toHaveAttribute('aria-sort', 'descending');
    });

    it('shows the ChevronDown icon', () => {
      const { container } = renderInTable(
        <SortableHeader
          label="Name"
          sortKey="name"
          currentSort="name"
          currentDirection="desc"
          onSort={vi.fn()}
        />,
      );
      const svg = container.querySelector('svg');
      expect(svg).not.toHaveClass('opacity-30');
    });
  });

  it('fires onSort on each click', async () => {
    const user = userEvent.setup();
    const handleSort = vi.fn();

    renderInTable(
      <SortableHeader
        label="Status"
        sortKey="status"
        currentSort="status"
        currentDirection="asc"
        onSort={handleSort}
      />,
    );

    await user.click(screen.getByRole('button'));
    await user.click(screen.getByRole('button'));
    expect(handleSort).toHaveBeenCalledTimes(2);
    expect(handleSort).toHaveBeenCalledWith('status');
  });
});
