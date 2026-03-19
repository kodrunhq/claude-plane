import { describe, it, expect } from 'vitest';
import { screen } from '@testing-library/react';
import { EventsTable } from '../../../components/events/EventsTable';
import { renderWithProviders } from '../../../test/render';
import { buildEvent } from '../../../test/factories';

describe('EventsTable', () => {
  it('renders table headers', () => {
    renderWithProviders(<EventsTable events={[]} />);
    expect(screen.getByText('Event Type')).toBeInTheDocument();
    expect(screen.getByText('Timestamp')).toBeInTheDocument();
    expect(screen.getByText('Source')).toBeInTheDocument();
    expect(screen.getByText('Payload')).toBeInTheDocument();
  });

  it('renders event rows with correct data', () => {
    const events = [
      buildEvent({
        event_id: 'e-1',
        event_type: 'session.started',
        source: 'agent',
        payload: { machine: 'worker-1' },
      }),
      buildEvent({
        event_id: 'e-2',
        event_type: 'run.completed',
        source: 'server',
        payload: {},
      }),
    ];
    renderWithProviders(<EventsTable events={events} />);
    expect(screen.getByText('session.started')).toBeInTheDocument();
    expect(screen.getByText('run.completed')).toBeInTheDocument();
    expect(screen.getByText('agent')).toBeInTheDocument();
    expect(screen.getByText('server')).toBeInTheDocument();
  });

  it('displays event type in a badge', () => {
    const events = [
      buildEvent({ event_id: 'e-1', event_type: 'job.triggered' }),
    ];
    renderWithProviders(<EventsTable events={events} />);
    const badge = screen.getByText('job.triggered');
    expect(badge.className).toContain('font-mono');
  });

  it('displays em dash when source is empty', () => {
    const events = [
      buildEvent({ event_id: 'e-1', source: '' }),
    ];
    renderWithProviders(<EventsTable events={events} />);
    expect(screen.getByText('\u2014')).toBeInTheDocument();
  });

  it('displays payload preview for events with payload', () => {
    const events = [
      buildEvent({
        event_id: 'e-1',
        payload: { key: 'value', count: 42 },
      }),
    ];
    renderWithProviders(<EventsTable events={events} />);
    expect(screen.getByText(/{"key":"value","count":42}/)).toBeInTheDocument();
  });

  it('displays {} for empty payload', () => {
    const events = [
      buildEvent({ event_id: 'e-1', payload: {} }),
    ];
    renderWithProviders(<EventsTable events={events} />);
    expect(screen.getByText('{}')).toBeInTheDocument();
  });

  it('renders expand button for events with payload', () => {
    const events = [
      buildEvent({
        event_id: 'e-1',
        payload: { data: 'present' },
      }),
    ];
    renderWithProviders(<EventsTable events={events} />);
    expect(screen.getByLabelText('Expand payload')).toBeInTheDocument();
  });

  it('does not render expand button for events with empty payload', () => {
    const events = [
      buildEvent({ event_id: 'e-1', payload: {} }),
    ];
    renderWithProviders(<EventsTable events={events} />);
    expect(screen.queryByLabelText('Expand payload')).not.toBeInTheDocument();
    expect(screen.queryByLabelText('Collapse payload')).not.toBeInTheDocument();
  });

  it('expands row to show formatted payload when expand button is clicked', async () => {
    const events = [
      buildEvent({
        event_id: 'e-1',
        payload: { detail: 'expanded view' },
      }),
    ];
    const { user } = renderWithProviders(<EventsTable events={events} />);
    await user.click(screen.getByLabelText('Expand payload'));
    expect(screen.getByLabelText('Collapse payload')).toBeInTheDocument();
    // The formatted JSON should be visible in the expanded row
    expect(screen.getByText(/"detail": "expanded view"/)).toBeInTheDocument();
  });

  it('collapses row when clicking expand button again', async () => {
    const events = [
      buildEvent({
        event_id: 'e-1',
        payload: { detail: 'toggle' },
      }),
    ];
    const { user } = renderWithProviders(<EventsTable events={events} />);
    await user.click(screen.getByLabelText('Expand payload'));
    expect(screen.getByLabelText('Collapse payload')).toBeInTheDocument();
    await user.click(screen.getByLabelText('Collapse payload'));
    expect(screen.getByLabelText('Expand payload')).toBeInTheDocument();
  });

  it('truncates long payload preview to 80 characters with ellipsis', () => {
    const longValue = 'x'.repeat(100);
    const events = [
      buildEvent({
        event_id: 'e-1',
        payload: { longField: longValue },
      }),
    ];
    renderWithProviders(<EventsTable events={events} />);
    // The preview should end with '...'
    const cells = screen.getAllByRole('cell');
    const payloadCell = cells.find((c) => c.textContent?.includes('...'));
    expect(payloadCell).toBeDefined();
  });

  it('formats timestamp for display', () => {
    const events = [
      buildEvent({
        event_id: 'e-1',
        timestamp: '2026-01-15T10:00:00Z',
      }),
    ];
    renderWithProviders(<EventsTable events={events} />);
    // Should render as a formatted date string (locale-dependent, but should contain 2026)
    const cells = screen.getAllByRole('cell');
    const timestampCell = cells.find((c) => c.textContent?.includes('2026'));
    expect(timestampCell).toBeDefined();
  });
});
