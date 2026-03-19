import { describe, it, expect, vi } from 'vitest';
import { renderWithProviders, screen } from '../../../test/render.tsx';
import { Pagination } from '../../../components/shared/Pagination.tsx';

describe('Pagination', () => {
  const defaultProps = {
    page: 1,
    pageSize: 25,
    total: 100,
    onPageChange: vi.fn(),
    onPageSizeChange: vi.fn(),
  };

  it('shows correct page info ("Page X of Y")', () => {
    renderWithProviders(
      <Pagination {...defaultProps} page={2} />,
    );

    // Pagination displays "X / Y" format
    expect(screen.getByText('2 / 4')).toBeInTheDocument();
  });

  it('shows correct showing range', () => {
    renderWithProviders(
      <Pagination {...defaultProps} page={2} />,
    );

    // Page 2 of 25 per page with 100 total: "Showing 26-50 of 100"
    expect(screen.getByText(/Showing 26/)).toBeInTheDocument();
    expect(screen.getByText(/of 100/)).toBeInTheDocument();
  });

  it('Previous button disabled on first page', () => {
    renderWithProviders(
      <Pagination {...defaultProps} page={1} />,
    );

    const prevButton = screen.getByText('Prev');
    expect(prevButton).toBeDisabled();
  });

  it('Next button disabled on last page', () => {
    renderWithProviders(
      <Pagination {...defaultProps} page={4} />,
    );

    const nextButton = screen.getByText('Next');
    expect(nextButton).toBeDisabled();
  });

  it('Previous button enabled when not on first page', () => {
    renderWithProviders(
      <Pagination {...defaultProps} page={2} />,
    );

    const prevButton = screen.getByText('Prev');
    expect(prevButton).toBeEnabled();
  });

  it('Next button enabled when not on last page', () => {
    renderWithProviders(
      <Pagination {...defaultProps} page={1} />,
    );

    const nextButton = screen.getByText('Next');
    expect(nextButton).toBeEnabled();
  });

  it('clicking Next calls onPageChange with incremented page', async () => {
    const onPageChange = vi.fn();
    const { user } = renderWithProviders(
      <Pagination {...defaultProps} page={2} onPageChange={onPageChange} />,
    );

    await user.click(screen.getByText('Next'));

    expect(onPageChange).toHaveBeenCalledWith(3);
  });

  it('clicking Previous calls onPageChange with decremented page', async () => {
    const onPageChange = vi.fn();
    const { user } = renderWithProviders(
      <Pagination {...defaultProps} page={3} onPageChange={onPageChange} />,
    );

    await user.click(screen.getByText('Prev'));

    expect(onPageChange).toHaveBeenCalledWith(2);
  });

  it('page size selector changes page size and resets to page 1', async () => {
    const onPageSizeChange = vi.fn();
    const onPageChange = vi.fn();
    const { user } = renderWithProviders(
      <Pagination
        {...defaultProps}
        page={3}
        onPageChange={onPageChange}
        onPageSizeChange={onPageSizeChange}
      />,
    );

    const pageSizeSelect = screen.getByLabelText('Rows per page');
    await user.selectOptions(pageSizeSelect, '50');

    expect(onPageSizeChange).toHaveBeenCalledWith(50);
    // Changing page size also resets to page 1
    expect(onPageChange).toHaveBeenCalledWith(1);
  });

  it('renders nothing when total is 0', () => {
    const { container } = renderWithProviders(
      <Pagination {...defaultProps} total={0} />,
    );

    expect(container.innerHTML).toBe('');
  });

  it('shows all page size options', () => {
    renderWithProviders(
      <Pagination {...defaultProps} />,
    );

    const pageSizeSelect = screen.getByLabelText('Rows per page');
    const options = pageSizeSelect.querySelectorAll('option');
    const values = Array.from(options).map((opt) => opt.textContent);

    expect(values).toContain('25 per page');
    expect(values).toContain('50 per page');
    expect(values).toContain('100 per page');
  });

  it('supports custom pageSizeOptions', () => {
    renderWithProviders(
      <Pagination {...defaultProps} pageSizeOptions={[10, 20]} />,
    );

    const pageSizeSelect = screen.getByLabelText('Rows per page');
    const options = pageSizeSelect.querySelectorAll('option');
    const values = Array.from(options).map((opt) => opt.textContent);

    expect(values).toContain('10 per page');
    expect(values).toContain('20 per page');
    expect(values).not.toContain('25 per page');
  });
});
