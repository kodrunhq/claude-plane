import { describe, it, expect, vi } from 'vitest';
import { http, HttpResponse } from 'msw';
import { server } from '../../../test/setup.ts';
import { renderWithProviders, screen, waitFor } from '../../../test/render.tsx';
import { LaunchTemplateModal } from '../../../components/templates/LaunchTemplateModal.tsx';
import { mockMachines } from '../../../test/handlers.ts';
import { buildTemplate, buildSession } from '../../../test/factories.ts';
import type { SessionTemplate } from '../../../types/template.ts';

// Mock react-router's useNavigate
const mockNavigate = vi.fn();
vi.mock('react-router', async () => {
  const actual = await vi.importActual('react-router');
  return {
    ...actual,
    useNavigate: () => mockNavigate,
  };
});

describe('LaunchTemplateModal', () => {
  const template: SessionTemplate = buildTemplate({
    template_id: 'tmpl-200',
    name: 'My Template',
    description: 'A test template',
    initial_prompt: '',
  });

  beforeEach(() => {
    mockNavigate.mockClear();
  });

  it('renders nothing when open is false', () => {
    renderWithProviders(
      <LaunchTemplateModal open={false} onClose={() => {}} template={template} />,
    );
    expect(screen.queryByText('Launch Template')).not.toBeInTheDocument();
  });

  it('renders nothing when template is null', () => {
    renderWithProviders(
      <LaunchTemplateModal open={true} onClose={() => {}} template={null} />,
    );
    expect(screen.queryByText('Launch Template')).not.toBeInTheDocument();
  });

  it('renders modal with template name when open', () => {
    renderWithProviders(
      <LaunchTemplateModal open={true} onClose={() => {}} template={template} />,
    );
    expect(screen.getByText('Launch Template')).toBeInTheDocument();
    expect(screen.getByText('My Template')).toBeInTheDocument();
  });

  it('renders Machine select dropdown', () => {
    renderWithProviders(
      <LaunchTemplateModal open={true} onClose={() => {}} template={template} />,
    );
    expect(screen.getByText('Machine')).toBeInTheDocument();
    expect(screen.getByDisplayValue('Select a machine...')).toBeInTheDocument();
  });

  it('populates machine dropdown with online machines from API', async () => {
    renderWithProviders(
      <LaunchTemplateModal open={true} onClose={() => {}} template={template} />,
    );

    const connectedMachine = mockMachines.find((m) => m.status === 'connected')!;

    await waitFor(() => {
      const select = screen.getByDisplayValue('Select a machine...');
      const options = select.querySelectorAll('option');
      const machineOption = Array.from(options).find(
        (opt) => opt.textContent === connectedMachine.display_name,
      );
      expect(machineOption).toBeTruthy();
    });
  });

  it('shows warning when no online machines are available', async () => {
    server.use(
      http.get('/api/v1/machines', () => HttpResponse.json([
        { machine_id: 'm1', display_name: 'Offline Worker', status: 'disconnected', max_sessions: 5, home_dir: '', last_seen_at: '', created_at: '' },
      ])),
    );

    renderWithProviders(
      <LaunchTemplateModal open={true} onClose={() => {}} template={template} />,
    );

    await waitFor(() => {
      expect(screen.getByText('No online machines available')).toBeInTheDocument();
    });
  });

  it('Launch button is disabled when no machine is selected', () => {
    renderWithProviders(
      <LaunchTemplateModal open={true} onClose={() => {}} template={template} />,
    );
    expect(screen.getByText('Launch')).toBeDisabled();
  });

  it('Launch button is enabled after selecting a machine', async () => {
    const { user } = renderWithProviders(
      <LaunchTemplateModal open={true} onClose={() => {}} template={template} />,
    );

    const connectedMachine = mockMachines.find((m) => m.status === 'connected')!;

    await waitFor(async () => {
      const select = screen.getByDisplayValue('Select a machine...');
      await user.selectOptions(select, connectedMachine.machine_id);
    });

    expect(screen.getByText('Launch')).toBeEnabled();
  });

  it('Cancel button calls onClose', async () => {
    const onClose = vi.fn();
    const { user } = renderWithProviders(
      <LaunchTemplateModal open={true} onClose={onClose} template={template} />,
    );

    await user.click(screen.getByText('Cancel'));
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('clicking backdrop calls onClose', async () => {
    const onClose = vi.fn();
    const { user } = renderWithProviders(
      <LaunchTemplateModal open={true} onClose={onClose} template={template} />,
    );

    // The backdrop is the absolute inset div
    const backdrop = screen.getByText('Launch Template').closest('.fixed')!.querySelector('.absolute');
    expect(backdrop).toBeTruthy();
    await user.click(backdrop!);
    expect(onClose).toHaveBeenCalled();
  });

  it('renders variable inputs when template has variables in prompt', () => {
    const templateWithVars = buildTemplate({
      template_id: 'tmpl-vars',
      name: 'Var Template',
      initial_prompt: 'Deploy ${BRANCH} to ${ENVIRONMENT}',
    });

    renderWithProviders(
      <LaunchTemplateModal open={true} onClose={() => {}} template={templateWithVars} />,
    );

    expect(screen.getByText('BRANCH')).toBeInTheDocument();
    expect(screen.getByText('ENVIRONMENT')).toBeInTheDocument();
    expect(screen.getByPlaceholderText('Enter value for BRANCH')).toBeInTheDocument();
    expect(screen.getByPlaceholderText('Enter value for ENVIRONMENT')).toBeInTheDocument();
  });

  it('does not render variable inputs when template has no variables', () => {
    renderWithProviders(
      <LaunchTemplateModal open={true} onClose={() => {}} template={template} />,
    );

    expect(screen.queryByPlaceholderText(/Enter value for/)).not.toBeInTheDocument();
  });

  it('submitting form calls createSession API with correct params', async () => {
    const createdSession = buildSession({ session_id: 'new-sess-tmpl', machine_id: 'machine-100' });
    let capturedBody: Record<string, unknown> | null = null;

    server.use(
      http.post('/api/v1/sessions', async ({ request }) => {
        capturedBody = await request.json() as Record<string, unknown>;
        return HttpResponse.json(createdSession, { status: 201 });
      }),
    );

    const onClose = vi.fn();
    const { user } = renderWithProviders(
      <LaunchTemplateModal open={true} onClose={onClose} template={template} />,
    );

    const connectedMachine = mockMachines.find((m) => m.status === 'connected')!;

    await waitFor(async () => {
      const select = screen.getByDisplayValue('Select a machine...');
      await user.selectOptions(select, connectedMachine.machine_id);
    });

    await user.click(screen.getByText('Launch'));

    await waitFor(() => {
      expect(capturedBody).not.toBeNull();
    });

    expect(capturedBody!.machine_id).toBe(connectedMachine.machine_id);
    expect(capturedBody!.template_id).toBe('tmpl-200');
  });

  it('submitting form with variables includes them in the request', async () => {
    const templateWithVars = buildTemplate({
      template_id: 'tmpl-vars-2',
      name: 'Var Template',
      initial_prompt: 'Deploy ${BRANCH}',
    });

    const createdSession = buildSession({ session_id: 'new-sess-vars', machine_id: 'machine-100' });
    let capturedBody: Record<string, unknown> | null = null;

    server.use(
      http.post('/api/v1/sessions', async ({ request }) => {
        capturedBody = await request.json() as Record<string, unknown>;
        return HttpResponse.json(createdSession, { status: 201 });
      }),
    );

    const { user } = renderWithProviders(
      <LaunchTemplateModal open={true} onClose={() => {}} template={templateWithVars} />,
    );

    const connectedMachine = mockMachines.find((m) => m.status === 'connected')!;

    await waitFor(async () => {
      const select = screen.getByDisplayValue('Select a machine...');
      await user.selectOptions(select, connectedMachine.machine_id);
    });

    const branchInput = screen.getByPlaceholderText('Enter value for BRANCH');
    await user.type(branchInput, 'feature-x');

    await user.click(screen.getByText('Launch'));

    await waitFor(() => {
      expect(capturedBody).not.toBeNull();
    });

    expect(capturedBody!.variables).toEqual({ BRANCH: 'feature-x' });
  });

  it('navigates to session page after successful launch', async () => {
    const createdSession = buildSession({ session_id: 'nav-sess-1', machine_id: 'machine-100' });

    server.use(
      http.post('/api/v1/sessions', () => HttpResponse.json(createdSession, { status: 201 })),
    );

    const onClose = vi.fn();
    const { user } = renderWithProviders(
      <LaunchTemplateModal open={true} onClose={onClose} template={template} />,
    );

    const connectedMachine = mockMachines.find((m) => m.status === 'connected')!;

    await waitFor(async () => {
      const select = screen.getByDisplayValue('Select a machine...');
      await user.selectOptions(select, connectedMachine.machine_id);
    });

    await user.click(screen.getByText('Launch'));

    await waitFor(() => {
      expect(mockNavigate).toHaveBeenCalledWith(`/sessions/${createdSession.session_id}`);
    });
    expect(onClose).toHaveBeenCalled();
  });
});
