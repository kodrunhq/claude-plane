import { describe, it, expect, vi, beforeEach } from 'vitest';
import { screen } from '@testing-library/react';
import { EventFilters, type EventFilterValues } from '../../../components/events/EventFilters';
import { renderWithProviders } from '../../../test/render';

const defaultFilters: EventFilterValues = {
  typePattern: '',
  since: '',
  limit: 25,
};

describe('EventFilters', () => {
  let onChange: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    onChange = vi.fn();
  });

  it('renders the type pattern input with placeholder', () => {
    renderWithProviders(
      <EventFilters filters={defaultFilters} onChange={onChange} />,
    );
    expect(
      screen.getByPlaceholderText('Filter by event type (e.g. run.*)'),
    ).toBeInTheDocument();
  });

  it('renders the since datetime-local input', () => {
    renderWithProviders(
      <EventFilters filters={defaultFilters} onChange={onChange} />,
    );
    expect(screen.getByText('Since')).toBeInTheDocument();
    expect(document.querySelector('input[type="datetime-local"]')).toBeInTheDocument();
  });

  it('renders the per-page select with limit options', () => {
    renderWithProviders(
      <EventFilters filters={defaultFilters} onChange={onChange} />,
    );
    expect(screen.getByText('Per page')).toBeInTheDocument();
    expect(screen.getByText('25')).toBeInTheDocument();
    expect(screen.getByText('50')).toBeInTheDocument();
    expect(screen.getByText('100')).toBeInTheDocument();
  });

  it('displays current filter values', () => {
    const filters: EventFilterValues = {
      typePattern: 'session.*',
      since: '2026-01-15T10:00',
      limit: 50,
    };
    renderWithProviders(
      <EventFilters filters={filters} onChange={onChange} />,
    );
    expect(screen.getByDisplayValue('session.*')).toBeInTheDocument();
    expect(screen.getByDisplayValue('2026-01-15T10:00')).toBeInTheDocument();
    const perPageSelect = document.querySelector('select') as HTMLSelectElement;
    expect(perPageSelect.value).toBe('50');
  });

  it('calls onChange with updated typePattern when type input changes', async () => {
    const { user } = renderWithProviders(
      <EventFilters filters={defaultFilters} onChange={onChange} />,
    );
    const input = screen.getByPlaceholderText(
      'Filter by event type (e.g. run.*)',
    );
    await user.type(input, 'run');
    // Each keystroke fires onChange; since the component is controlled with static
    // defaultFilters (typePattern: ''), each keystroke sees an empty field,
    // so each call gets a single-character typePattern.
    expect(onChange).toHaveBeenCalledTimes(3);
    expect(onChange.mock.calls[0][0].typePattern).toBe('r');
  });

  it('calls onChange with updated limit when per-page select changes', async () => {
    const { user } = renderWithProviders(
      <EventFilters filters={defaultFilters} onChange={onChange} />,
    );
    const selectEl = document.querySelector('select') as HTMLSelectElement;
    await user.selectOptions(selectEl, '100');
    expect(onChange).toHaveBeenCalledWith({
      ...defaultFilters,
      limit: 100,
    });
  });

  it('calls onChange with updated since when datetime input changes', async () => {
    const { user } = renderWithProviders(
      <EventFilters filters={defaultFilters} onChange={onChange} />,
    );
    const input = document.querySelector('input[type="datetime-local"]') as HTMLInputElement;
    await user.type(input, '2026-03-19T12:00');
    expect(onChange).toHaveBeenCalled();
  });
});
