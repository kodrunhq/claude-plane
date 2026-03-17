import { describe, it, expect } from 'vitest';
import { http, HttpResponse } from 'msw';
import { server } from '../../test/setup.ts';
import { renderWithProviders, screen, waitFor, userEvent } from '../../test/render.tsx';
import { JobsPage } from '../../views/JobsPage.tsx';
import { mockJobs } from '../../test/handlers.ts';

describe('JobsPage', () => {
  it('renders page heading and New Job button', () => {
    renderWithProviders(<JobsPage />);

    expect(screen.getByText('Jobs')).toBeInTheDocument();
    expect(screen.getByText('New Job')).toBeInTheDocument();
  });

  it('renders jobs table with job names from API', async () => {
    renderWithProviders(<JobsPage />);

    await waitFor(() => {
      expect(screen.getByText(mockJobs[0].name)).toBeInTheDocument();
      expect(screen.getByText(mockJobs[1].name)).toBeInTheDocument();
    });
  });

  it('shows job description in the table', async () => {
    renderWithProviders(<JobsPage />);

    await waitFor(() => {
      // Both mock jobs have description 'A test job' from the factory
      const descriptions = screen.getAllByText('A test job');
      expect(descriptions.length).toBeGreaterThanOrEqual(1);
    });
  });

  it('renders search input', () => {
    renderWithProviders(<JobsPage />);

    expect(screen.getByPlaceholderText('Search by name or ID...')).toBeInTheDocument();
  });

  it('filters jobs by search query', async () => {
    renderWithProviders(<JobsPage />);
    const user = userEvent.setup();

    // Wait for jobs to load
    await waitFor(() => {
      expect(screen.getByText(mockJobs[0].name)).toBeInTheDocument();
    });

    // Type in the search input to filter
    const searchInput = screen.getByPlaceholderText('Search by name or ID...');
    await user.type(searchInput, 'Deploy');

    // Only "Deploy Frontend" should remain visible
    expect(screen.getByText('Deploy Frontend')).toBeInTheDocument();
    expect(screen.queryByText('Run Tests')).not.toBeInTheDocument();
  });

  it('shows empty state when no jobs exist', async () => {
    server.use(
      http.get('/api/v1/jobs', () => HttpResponse.json([])),
    );

    renderWithProviders(<JobsPage />);

    await waitFor(() => {
      expect(screen.getByText('No jobs yet')).toBeInTheDocument();
    });
  });

  it('shows Run button for each job', async () => {
    renderWithProviders(<JobsPage />);

    await waitFor(() => {
      const runButtons = screen.getAllByText('Run');
      expect(runButtons.length).toBe(mockJobs.length);
    });
  });

  it('shows delete button for each job', async () => {
    renderWithProviders(<JobsPage />);

    await waitFor(() => {
      const deleteButtons = screen.getAllByTitle('Delete');
      expect(deleteButtons.length).toBe(mockJobs.length);
    });
  });
});
