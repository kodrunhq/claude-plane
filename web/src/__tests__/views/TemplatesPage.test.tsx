import { describe, it, expect } from 'vitest';
import { http, HttpResponse } from 'msw';
import { server } from '../../test/setup.ts';
import { renderWithProviders, screen, waitFor } from '../../test/render.tsx';
import { TemplatesPage } from '../../views/TemplatesPage.tsx';
import { buildTemplate, buildMachine } from '../../test/factories.ts';

const mockTemplates = [
  buildTemplate({ template_id: 'tmpl-200', name: 'Claude Default', command: 'claude' }),
  buildTemplate({ template_id: 'tmpl-201', name: 'Bash Runner', command: 'bash' }),
];

const mockMachines = [
  buildMachine({ machine_id: 'machine-200', display_name: 'Worker A', status: 'connected' }),
];

function setupHandlers() {
  server.use(
    http.get('/api/v1/templates', () => HttpResponse.json(mockTemplates)),
    http.get('/api/v1/machines', () => HttpResponse.json(mockMachines)),
  );
}

describe('TemplatesPage', () => {
  it('renders template rows with Launch buttons', async () => {
    setupHandlers();
    renderWithProviders(<TemplatesPage />);

    await waitFor(() => {
      expect(screen.getByText('Claude Default')).toBeInTheDocument();
    });

    expect(screen.getByText('Bash Runner')).toBeInTheDocument();

    // Verify Launch buttons exist (one per template)
    const launchButtons = screen.getAllByTitle('Launch');
    expect(launchButtons).toHaveLength(2);
  });

  it('opens LaunchTemplateModal when Launch button is clicked', async () => {
    setupHandlers();
    const { user } = renderWithProviders(<TemplatesPage />);

    await waitFor(() => {
      expect(screen.getByText('Claude Default')).toBeInTheDocument();
    });

    const launchButtons = screen.getAllByTitle('Launch');
    await user.click(launchButtons[0]);

    // The modal should appear with template name and machine select
    await waitFor(() => {
      expect(screen.getByText('Launch Template')).toBeInTheDocument();
    });
  });

  it('does not show tag filter dropdown', async () => {
    server.use(
      http.get('/api/v1/templates', () =>
        HttpResponse.json([
          buildTemplate({ template_id: 'tmpl-300', name: 'Tagged', tags: ['devops', 'ci'] }),
        ]),
      ),
    );

    renderWithProviders(<TemplatesPage />);

    await waitFor(() => {
      expect(screen.getByText('Tagged')).toBeInTheDocument();
    });

    // Tag filter dropdown should not exist
    expect(screen.queryByText('All Tags')).not.toBeInTheDocument();
  });

  it('renders Duplicate and Delete buttons alongside Launch', async () => {
    setupHandlers();
    renderWithProviders(<TemplatesPage />);

    await waitFor(() => {
      expect(screen.getByText('Claude Default')).toBeInTheDocument();
    });

    expect(screen.getAllByTitle('Duplicate')).toHaveLength(2);
    expect(screen.getAllByTitle('Delete')).toHaveLength(2);
    expect(screen.getAllByTitle('Launch')).toHaveLength(2);
  });
});
