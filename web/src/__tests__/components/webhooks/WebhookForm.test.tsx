import { describe, it, expect, vi } from 'vitest';
import { renderWithProviders, screen } from '../../../test/render.tsx';
import { WebhookForm } from '../../../components/webhooks/WebhookForm.tsx';
import { EVENT_GROUPS } from '../../../constants/eventTypes.ts';
import type { Webhook, CreateWebhookParams } from '../../../types/webhook.ts';

describe('WebhookForm', () => {
  const defaultProps = {
    onSubmit: vi.fn(),
    onCancel: vi.fn(),
    submitting: false,
  };

  function renderForm(overrides?: Partial<typeof defaultProps> & { initial?: Webhook }) {
    const props = { ...defaultProps, ...overrides };
    return renderWithProviders(<WebhookForm {...props} />);
  }

  it('renders Name input field', () => {
    renderForm();
    expect(screen.getByPlaceholderText('My webhook')).toBeInTheDocument();
  });

  it('renders Endpoint URL input field', () => {
    renderForm();
    expect(screen.getByPlaceholderText('https://example.com/webhook')).toBeInTheDocument();
  });

  it('renders Secret input field', () => {
    renderForm();
    expect(screen.getByPlaceholderText('HMAC signing secret')).toBeInTheDocument();
  });

  it('renders Events section with group labels', () => {
    renderForm();
    expect(screen.getByText('Events')).toBeInTheDocument();
    // Check that event group labels are rendered
    for (const group of EVENT_GROUPS) {
      expect(screen.getByText(group.label)).toBeInTheDocument();
    }
  });

  it('renders individual event checkboxes', () => {
    renderForm();
    const allEvents = EVENT_GROUPS.flatMap((g) => g.events);
    // Check a few representative events exist
    expect(screen.getByText('run.created')).toBeInTheDocument();
    expect(screen.getByText('session.started')).toBeInTheDocument();
    expect(screen.getByText('machine.connected')).toBeInTheDocument();
    // Total checkboxes = group checkboxes + individual event checkboxes
    const checkboxes = screen.getAllByRole('checkbox');
    // Groups + individual events
    expect(checkboxes.length).toBe(EVENT_GROUPS.length + allEvents.length);
  });

  it('renders Enabled toggle defaulting to on', () => {
    renderForm();
    const toggle = screen.getByRole('switch');
    expect(toggle).toHaveAttribute('aria-checked', 'true');
  });

  it('renders Cancel and Create webhook buttons', () => {
    renderForm();
    expect(screen.getByText('Cancel')).toBeInTheDocument();
    expect(screen.getByText('Create webhook')).toBeInTheDocument();
  });

  it('name input accepts text', async () => {
    const { user } = renderForm();
    const input = screen.getByPlaceholderText('My webhook');
    await user.type(input, 'Deploy Hook');
    expect(input).toHaveValue('Deploy Hook');
  });

  it('URL input accepts text', async () => {
    const { user } = renderForm();
    const input = screen.getByPlaceholderText('https://example.com/webhook');
    await user.type(input, 'https://hooks.example.com/deploy');
    expect(input).toHaveValue('https://hooks.example.com/deploy');
  });

  it('secret input accepts text and has type password', async () => {
    const { user } = renderForm();
    const input = screen.getByPlaceholderText('HMAC signing secret');
    expect(input).toHaveAttribute('type', 'password');
    await user.type(input, 'mysecret');
    expect(input).toHaveValue('mysecret');
  });

  it('clicking an event checkbox toggles it', async () => {
    const { user } = renderForm();
    const runCreatedCheckbox = screen.getByText('run.created').closest('label')!.querySelector('input')!;
    expect(runCreatedCheckbox).not.toBeChecked();

    await user.click(runCreatedCheckbox);
    expect(runCreatedCheckbox).toBeChecked();

    await user.click(runCreatedCheckbox);
    expect(runCreatedCheckbox).not.toBeChecked();
  });

  it('clicking a group checkbox toggles all events in that group', async () => {
    const { user } = renderForm();
    const runsGroup = EVENT_GROUPS.find((g) => g.label === 'Runs')!;

    // Click the "Runs" group checkbox
    const groupCheckbox = screen.getByText('Runs').closest('label')!.querySelector('input')!;
    await user.click(groupCheckbox);

    // All run events should now be checked
    for (const event of runsGroup.events) {
      const checkbox = screen.getByText(event).closest('label')!.querySelector('input')!;
      expect(checkbox).toBeChecked();
    }

    // Click again to deselect all
    await user.click(groupCheckbox);
    for (const event of runsGroup.events) {
      const checkbox = screen.getByText(event).closest('label')!.querySelector('input')!;
      expect(checkbox).not.toBeChecked();
    }
  });

  it('Select all / Deselect all button works', async () => {
    const { user } = renderForm();

    // Initially shows "Select all"
    expect(screen.getByText('Select all')).toBeInTheDocument();

    await user.click(screen.getByText('Select all'));

    // Now all checkboxes should be checked and button says "Deselect all"
    expect(screen.getByText('Deselect all')).toBeInTheDocument();

    await user.click(screen.getByText('Deselect all'));

    expect(screen.getByText('Select all')).toBeInTheDocument();
  });

  it('Enabled toggle can be switched off', async () => {
    const { user } = renderForm();
    const toggle = screen.getByRole('switch');
    expect(toggle).toHaveAttribute('aria-checked', 'true');

    await user.click(toggle);
    expect(toggle).toHaveAttribute('aria-checked', 'false');
  });

  it('Cancel button calls onCancel', async () => {
    const onCancel = vi.fn();
    const { user } = renderForm({ onCancel });
    await user.click(screen.getByText('Cancel'));
    expect(onCancel).toHaveBeenCalledTimes(1);
  });

  it('shows validation error for empty name on submit', async () => {
    const onSubmit = vi.fn();
    const { user } = renderForm({ onSubmit });

    // Fill URL and select an event but leave name empty
    await user.type(screen.getByPlaceholderText('https://example.com/webhook'), 'https://example.com');
    const firstEvent = screen.getByText('run.created').closest('label')!.querySelector('input')!;
    await user.click(firstEvent);

    await user.click(screen.getByText('Create webhook'));

    expect(screen.getByText('Name is required')).toBeInTheDocument();
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it('shows validation error for empty URL on submit', async () => {
    const onSubmit = vi.fn();
    const { user } = renderForm({ onSubmit });

    // Fill name and select an event but leave URL empty
    await user.type(screen.getByPlaceholderText('My webhook'), 'Test Hook');
    const firstEvent = screen.getByText('run.created').closest('label')!.querySelector('input')!;
    await user.click(firstEvent);

    await user.click(screen.getByText('Create webhook'));

    expect(screen.getByText('URL is required')).toBeInTheDocument();
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it('shows validation error for invalid URL on submit', async () => {
    const onSubmit = vi.fn();
    const { user } = renderForm({ onSubmit });

    await user.type(screen.getByPlaceholderText('My webhook'), 'Test Hook');
    const urlInput = screen.getByPlaceholderText('https://example.com/webhook');
    await user.type(urlInput, 'not-a-url');
    const firstEvent = screen.getByText('run.created').closest('label')!.querySelector('input')!;
    await user.click(firstEvent);

    await user.click(screen.getByText('Create webhook'));

    // The input has type="url", so native browser validation prevents form submission
    // for invalid URLs before the custom validation can run
    expect((urlInput as HTMLInputElement).validity.valid).toBe(false);
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it('shows validation error when no events selected on submit', async () => {
    const onSubmit = vi.fn();
    const { user } = renderForm({ onSubmit });

    await user.type(screen.getByPlaceholderText('My webhook'), 'Test Hook');
    await user.type(screen.getByPlaceholderText('https://example.com/webhook'), 'https://example.com/hook');

    await user.click(screen.getByText('Create webhook'));

    expect(screen.getByText('Select at least one event')).toBeInTheDocument();
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it('calls onSubmit with correct params on valid form', async () => {
    const onSubmit = vi.fn();
    const { user } = renderForm({ onSubmit });

    await user.type(screen.getByPlaceholderText('My webhook'), 'Deploy Hook');
    await user.type(screen.getByPlaceholderText('https://example.com/webhook'), 'https://hooks.example.com');
    await user.type(screen.getByPlaceholderText('HMAC signing secret'), 'secret123');

    const firstEvent = screen.getByText('run.created').closest('label')!.querySelector('input')!;
    await user.click(firstEvent);

    await user.click(screen.getByText('Create webhook'));

    expect(onSubmit).toHaveBeenCalledWith(expect.objectContaining({
      name: 'Deploy Hook',
      url: 'https://hooks.example.com',
      secret: 'secret123',
      events: ['run.created'],
      enabled: true,
    }));
  });

  it('does not include secret in params when empty', async () => {
    const onSubmit = vi.fn();
    const { user } = renderForm({ onSubmit });

    await user.type(screen.getByPlaceholderText('My webhook'), 'No Secret Hook');
    await user.type(screen.getByPlaceholderText('https://example.com/webhook'), 'https://hooks.example.com');

    const firstEvent = screen.getByText('run.created').closest('label')!.querySelector('input')!;
    await user.click(firstEvent);

    await user.click(screen.getByText('Create webhook'));

    const calledWith = onSubmit.mock.calls[0][0] as CreateWebhookParams;
    expect(calledWith.secret).toBeUndefined();
  });

  it('submit button is disabled when submitting', () => {
    renderForm({ submitting: true });
    expect(screen.getByText('Saving...')).toBeDisabled();
  });

  it('shows "Saving..." when submitting', () => {
    renderForm({ submitting: true });
    expect(screen.getByText('Saving...')).toBeInTheDocument();
  });

  it('shows "Update webhook" when editing (initial provided)', () => {
    const existing: Webhook = {
      webhook_id: 'wh-1',
      name: 'Existing Hook',
      url: 'https://existing.com/hook',
      events: ['run.created'],
      enabled: true,
      created_at: '2026-01-15T10:00:00Z',
      updated_at: '2026-01-15T10:00:00Z',
    };

    renderForm({ initial: existing });
    expect(screen.getByText('Update webhook')).toBeInTheDocument();
  });

  it('pre-fills form when editing with initial values', () => {
    const existing: Webhook = {
      webhook_id: 'wh-1',
      name: 'Existing Hook',
      url: 'https://existing.com/hook',
      events: ['run.created', 'session.started'],
      enabled: true,
      created_at: '2026-01-15T10:00:00Z',
      updated_at: '2026-01-15T10:00:00Z',
    };

    renderForm({ initial: existing });
    expect(screen.getByDisplayValue('Existing Hook')).toBeInTheDocument();
    expect(screen.getByDisplayValue('https://existing.com/hook')).toBeInTheDocument();

    // Pre-selected events should be checked
    const runCreatedCheckbox = screen.getByText('run.created').closest('label')!.querySelector('input')!;
    expect(runCreatedCheckbox).toBeChecked();
    const sessionStartedCheckbox = screen.getByText('session.started').closest('label')!.querySelector('input')!;
    expect(sessionStartedCheckbox).toBeChecked();
  });

  it('secret field shows "(leave blank to keep existing)" hint when editing', () => {
    const existing: Webhook = {
      webhook_id: 'wh-1',
      name: 'Existing Hook',
      url: 'https://existing.com/hook',
      events: ['run.created'],
      enabled: true,
      created_at: '2026-01-15T10:00:00Z',
      updated_at: '2026-01-15T10:00:00Z',
    };

    renderForm({ initial: existing });
    expect(screen.getByText('(leave blank to keep existing)')).toBeInTheDocument();
  });

  it('secret field shows "(optional)" hint when creating', () => {
    renderForm();
    expect(screen.getByText('(optional)')).toBeInTheDocument();
  });
});
