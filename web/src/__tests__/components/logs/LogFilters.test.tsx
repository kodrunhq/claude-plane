import { describe, it, expect, vi, beforeEach } from 'vitest';
import { screen } from '@testing-library/react';
import { LogFilters } from '../../../components/logs/LogFilters';
import { renderWithProviders } from '../../../test/render';
import { useLogsStore } from '../../../stores/logs';

/** Find a <select> by looking for a <label> with the given text and grabbing the sibling select. */
function getSelectByLabel(labelText: string): HTMLSelectElement {
  const label = screen.getByText(labelText, { selector: 'label' });
  const select = label.parentElement?.querySelector('select');
  if (!select) throw new Error(`No select found for label "${labelText}"`);
  return select as HTMLSelectElement;
}

describe('LogFilters', () => {
  beforeEach(() => {
    useLogsStore.setState({
      filter: { limit: 100, offset: 0 },
      live: false,
    });
  });

  it('renders level filter dropdown', () => {
    renderWithProviders(<LogFilters />);
    const select = getSelectByLabel('Level');
    expect(select.value).toBe('ALL');
  });

  it('renders source filter dropdown', () => {
    renderWithProviders(<LogFilters />);
    expect(screen.getByText('Source', { selector: 'label' })).toBeInTheDocument();
  });

  it('renders component filter dropdown', () => {
    renderWithProviders(<LogFilters />);
    expect(screen.getByText('Component', { selector: 'label' })).toBeInTheDocument();
  });

  it('renders machine ID input', () => {
    renderWithProviders(<LogFilters />);
    expect(screen.getByPlaceholderText('Machine ID')).toBeInTheDocument();
  });

  it('renders search input', () => {
    renderWithProviders(<LogFilters />);
    expect(screen.getByPlaceholderText('Search log messages...')).toBeInTheDocument();
  });

  it('renders time preset buttons', () => {
    renderWithProviders(<LogFilters />);
    expect(screen.getByText('1h')).toBeInTheDocument();
    expect(screen.getByText('6h')).toBeInTheDocument();
    expect(screen.getByText('24h')).toBeInTheDocument();
    expect(screen.getByText('7d')).toBeInTheDocument();
  });

  it('renders Live toggle button', () => {
    renderWithProviders(<LogFilters />);
    expect(screen.getByText('Live')).toBeInTheDocument();
  });

  it('calls setFilter when level is changed', async () => {
    const setFilter = vi.fn();
    useLogsStore.setState({ setFilter });
    const { user } = renderWithProviders(<LogFilters />);
    await user.selectOptions(getSelectByLabel('Level'), 'ERROR');
    expect(setFilter).toHaveBeenCalledWith({ level: 'ERROR' });
  });

  it('calls setFilter with undefined level when ALL is selected', async () => {
    useLogsStore.setState({
      filter: { level: 'ERROR', limit: 100, offset: 0 },
    });
    const setFilter = vi.fn();
    useLogsStore.setState({ setFilter });
    const { user } = renderWithProviders(<LogFilters />);
    await user.selectOptions(getSelectByLabel('Level'), 'ALL');
    expect(setFilter).toHaveBeenCalledWith({ level: undefined });
  });

  it('calls setFilter when source is changed', async () => {
    const setFilter = vi.fn();
    useLogsStore.setState({ setFilter });
    const { user } = renderWithProviders(<LogFilters />);
    await user.selectOptions(getSelectByLabel('Source'), 'agent');
    expect(setFilter).toHaveBeenCalledWith({ source: 'agent' });
  });

  it('calls setFilter when component is changed', async () => {
    const setFilter = vi.fn();
    useLogsStore.setState({ setFilter });
    const { user } = renderWithProviders(<LogFilters />);
    await user.selectOptions(getSelectByLabel('Component'), 'grpc');
    expect(setFilter).toHaveBeenCalledWith({ component: 'grpc' });
  });

  it('calls setFilter when search input changes', async () => {
    const setFilter = vi.fn();
    useLogsStore.setState({ setFilter });
    const { user } = renderWithProviders(<LogFilters />);
    await user.type(screen.getByPlaceholderText('Search log messages...'), 'err');
    expect(setFilter).toHaveBeenCalled();
  });

  it('calls setFilter when machine ID input changes', async () => {
    const setFilter = vi.fn();
    useLogsStore.setState({ setFilter });
    const { user } = renderWithProviders(<LogFilters />);
    await user.type(screen.getByPlaceholderText('Machine ID'), 'worker-1');
    expect(setFilter).toHaveBeenCalled();
  });

  it('calls setFilter with since when time preset button is clicked', async () => {
    const setFilter = vi.fn();
    useLogsStore.setState({ setFilter });
    const { user } = renderWithProviders(<LogFilters />);
    await user.click(screen.getByText('1h'));
    expect(setFilter).toHaveBeenCalledWith(
      expect.objectContaining({ since: expect.any(String), until: undefined }),
    );
  });

  it('calls setLive when Live button is clicked', async () => {
    const setLive = vi.fn();
    useLogsStore.setState({ setLive });
    const { user } = renderWithProviders(<LogFilters />);
    await user.click(screen.getByText('Live'));
    expect(setLive).toHaveBeenCalledWith(true);
  });

  it('calls setLive(false) when Live button is clicked while live is active', async () => {
    const setLive = vi.fn();
    useLogsStore.setState({ live: true, setLive });
    const { user } = renderWithProviders(<LogFilters />);
    await user.click(screen.getByText('Live'));
    expect(setLive).toHaveBeenCalledWith(false);
  });

  it('displays current filter values in inputs', () => {
    useLogsStore.setState({
      filter: {
        level: 'WARN',
        source: 'agent',
        component: 'session',
        machine_id: 'worker-5',
        search: 'timeout',
        limit: 100,
        offset: 0,
      },
    });
    renderWithProviders(<LogFilters />);
    expect(getSelectByLabel('Level').value).toBe('WARN');
    expect(getSelectByLabel('Source').value).toBe('agent');
    expect(getSelectByLabel('Component').value).toBe('session');
    expect(screen.getByDisplayValue('worker-5')).toBeInTheDocument();
    expect(screen.getByDisplayValue('timeout')).toBeInTheDocument();
  });

  it('renders all level options', () => {
    renderWithProviders(<LogFilters />);
    const select = getSelectByLabel('Level');
    const options = Array.from(select.options).map((o) => o.value);
    expect(options).toEqual(['ALL', 'DEBUG', 'INFO', 'WARN', 'ERROR']);
  });

  it('renders all source options', () => {
    renderWithProviders(<LogFilters />);
    const select = getSelectByLabel('Source');
    const options = Array.from(select.options).map((o) => o.value);
    expect(options).toEqual(['ALL', 'server', 'agent']);
  });

  it('renders all component options', () => {
    renderWithProviders(<LogFilters />);
    const select = getSelectByLabel('Component');
    const options = Array.from(select.options).map((o) => o.value);
    expect(options).toEqual([
      'ALL', 'grpc', 'session', 'connmgr', 'auth', 'orchestrator', 'scheduler', 'event',
    ]);
  });
});
