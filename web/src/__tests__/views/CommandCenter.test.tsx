import { describe, it, expect } from 'vitest';
import { renderWithProviders, screen, waitFor } from '../../test/render.tsx';
import { CommandCenter } from '../../views/CommandCenter.tsx';

describe('CommandCenter', () => {
  it('renders the Command Center heading', () => {
    renderWithProviders(<CommandCenter />);

    expect(screen.getByRole('heading', { name: 'Command Center' })).toBeInTheDocument();
  });

  it('renders stat card labels after data loads', async () => {
    renderWithProviders(<CommandCenter />);

    // Some labels appear both as stat card text and section headings, so use getAllByText
    await waitFor(() => {
      expect(screen.getAllByText(/Active Sessions/).length).toBeGreaterThanOrEqual(1);
      expect(screen.getByText('Online Machines')).toBeInTheDocument();
      expect(screen.getByText('Total Jobs')).toBeInTheDocument();
      expect(screen.getAllByText(/Recent Runs/).length).toBeGreaterThanOrEqual(1);
      expect(screen.getByText('Completion Rate')).toBeInTheDocument();
    });
  });

  it('renders section headings for Machines, Jobs, and Runs', async () => {
    renderWithProviders(<CommandCenter />);

    await waitFor(() => {
      // Section headings are h2 elements
      const headings = screen.getAllByRole('heading', { level: 2 });
      const headingTexts = headings.map((h) => h.textContent);
      expect(headingTexts).toContain('Machines');
      expect(headingTexts).toContain('Recent Jobs');
      expect(headingTexts).toContain('Recent Runs');
    });
  });

  it('shows machine and session data from API after loading', async () => {
    renderWithProviders(<CommandCenter />);

    // Wait for the data to load -- stat values should no longer be '--'
    await waitFor(() => {
      const statValues = screen.getAllByText('1');
      expect(statValues.length).toBeGreaterThanOrEqual(1);
    });
  });

  it('renders New Session button', () => {
    renderWithProviders(<CommandCenter />);

    expect(screen.getByText('New Session')).toBeInTheDocument();
  });

  it('renders View All links', async () => {
    renderWithProviders(<CommandCenter />);

    await waitFor(() => {
      expect(screen.getByText('View All Machines')).toBeInTheDocument();
      expect(screen.getByText('View All Sessions')).toBeInTheDocument();
      expect(screen.getByText('View All Jobs')).toBeInTheDocument();
      expect(screen.getByText('View All Runs')).toBeInTheDocument();
    });
  });
});
