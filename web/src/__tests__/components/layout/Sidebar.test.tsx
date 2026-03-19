import { describe, it, expect, vi } from 'vitest';
import { renderWithProviders, screen } from '../../../test/render.tsx';
import { Sidebar } from '../../../components/layout/Sidebar.tsx';
import { useAuthStore } from '../../../stores/auth.ts';
import { useUIStore } from '../../../stores/ui.ts';

describe('Sidebar', () => {
  beforeEach(() => {
    // Reset stores to defaults
    useAuthStore.setState({
      user: { userId: 'u1', email: 'admin@test.com', displayName: 'Admin', role: 'admin' },
      authenticated: true,
      loading: false,
    });
    useUIStore.setState({ sidebarCollapsed: false });
  });

  it('renders Core nav section links', () => {
    renderWithProviders(<Sidebar />);
    expect(screen.getByText('Command Center')).toBeInTheDocument();
    expect(screen.getByText('Sessions')).toBeInTheDocument();
    expect(screen.getByText('Multi-View')).toBeInTheDocument();
    expect(screen.getByText('Machines')).toBeInTheDocument();
    expect(screen.getByText('Jobs')).toBeInTheDocument();
    expect(screen.getByText('Templates')).toBeInTheDocument();
    expect(screen.getByText('Runs')).toBeInTheDocument();
    expect(screen.getByText('Search')).toBeInTheDocument();
  });

  it('renders Automation nav section links', () => {
    renderWithProviders(<Sidebar />);
    expect(screen.getByText('Webhooks')).toBeInTheDocument();
    expect(screen.getByText('Triggers')).toBeInTheDocument();
    expect(screen.getByText('Schedules')).toBeInTheDocument();
    expect(screen.getByText('Connectors')).toBeInTheDocument();
  });

  it('renders Monitoring nav section links', () => {
    renderWithProviders(<Sidebar />);
    expect(screen.getByText('Events')).toBeInTheDocument();
    expect(screen.getByText('Logs')).toBeInTheDocument();
    expect(screen.getByText('Credentials')).toBeInTheDocument();
  });

  it('renders Admin nav section for admin users', () => {
    renderWithProviders(<Sidebar />);
    expect(screen.getByText('Users')).toBeInTheDocument();
    expect(screen.getByText('Provisioning')).toBeInTheDocument();
    expect(screen.getByText('API Keys')).toBeInTheDocument();
  });

  it('hides Admin section for non-admin users', () => {
    useAuthStore.setState({
      user: { userId: 'u2', email: 'user@test.com', displayName: 'User', role: 'user' },
    });

    renderWithProviders(<Sidebar />);
    expect(screen.queryByText('Users')).not.toBeInTheDocument();
    expect(screen.queryByText('Provisioning')).not.toBeInTheDocument();
    expect(screen.queryByText('API Keys')).not.toBeInTheDocument();
  });

  it('renders Help section with Documentation link', () => {
    renderWithProviders(<Sidebar />);
    expect(screen.getByText('Documentation')).toBeInTheDocument();
  });

  it('renders Settings link', () => {
    renderWithProviders(<Sidebar />);
    expect(screen.getByText('Settings')).toBeInTheDocument();
  });

  it('renders Sign out button', () => {
    renderWithProviders(<Sidebar />);
    expect(screen.getByLabelText('Sign out')).toBeInTheDocument();
    expect(screen.getByText('Sign out')).toBeInTheDocument();
  });

  it('Sign out button calls logout', async () => {
    const { user } = renderWithProviders(<Sidebar />);
    const logoutSpy = vi.fn();
    useAuthStore.setState({ logout: logoutSpy });

    await user.click(screen.getByLabelText('Sign out'));
    expect(logoutSpy).toHaveBeenCalled();
  });

  it('nav links point to correct routes', () => {
    renderWithProviders(<Sidebar />);
    const links = screen.getAllByRole('link');
    const hrefs = links.map((l) => l.getAttribute('href'));

    expect(hrefs).toContain('/');
    expect(hrefs).toContain('/sessions');
    expect(hrefs).toContain('/multiview');
    expect(hrefs).toContain('/machines');
    expect(hrefs).toContain('/jobs');
    expect(hrefs).toContain('/templates');
    expect(hrefs).toContain('/runs');
    expect(hrefs).toContain('/search');
    expect(hrefs).toContain('/webhooks');
    expect(hrefs).toContain('/triggers');
    expect(hrefs).toContain('/schedules');
    expect(hrefs).toContain('/connectors');
    expect(hrefs).toContain('/events');
    expect(hrefs).toContain('/logs');
    expect(hrefs).toContain('/credentials');
    expect(hrefs).toContain('/users');
    expect(hrefs).toContain('/provisioning');
    expect(hrefs).toContain('/api-keys');
    expect(hrefs).toContain('/docs');
    expect(hrefs).toContain('/settings');
  });

  it('active link gets active class when route matches', () => {
    renderWithProviders(<Sidebar />, { routes: ['/sessions'] });
    const sessionsLink = screen.getByText('Sessions').closest('a')!;
    expect(sessionsLink.className).toContain('text-accent-primary');
  });

  it('non-active links get secondary text class', () => {
    renderWithProviders(<Sidebar />, { routes: ['/sessions'] });
    const jobsLink = screen.getByText('Jobs').closest('a')!;
    expect(jobsLink.className).toContain('text-text-secondary');
  });

  it('section titles are rendered when sidebar is not collapsed', () => {
    renderWithProviders(<Sidebar />);
    expect(screen.getByText('Core')).toBeInTheDocument();
    expect(screen.getByText('Automation')).toBeInTheDocument();
    expect(screen.getByText('Monitoring')).toBeInTheDocument();
    expect(screen.getByText('Admin')).toBeInTheDocument();
    expect(screen.getByText('Help')).toBeInTheDocument();
  });

  it('section titles are hidden when sidebar is collapsed', () => {
    useUIStore.setState({ sidebarCollapsed: true });
    renderWithProviders(<Sidebar />);
    expect(screen.queryByText('Core')).not.toBeInTheDocument();
    expect(screen.queryByText('Automation')).not.toBeInTheDocument();
    expect(screen.queryByText('Monitoring')).not.toBeInTheDocument();
  });

  it('link labels are hidden when sidebar is collapsed', () => {
    useUIStore.setState({ sidebarCollapsed: true });
    renderWithProviders(<Sidebar />);
    // NavItemLink renders label in a <span> only when not collapsed
    expect(screen.queryByText('Command Center')).not.toBeInTheDocument();
    expect(screen.queryByText('Sessions')).not.toBeInTheDocument();
  });

  it('calls onNavigate callback when a nav link is clicked', async () => {
    const onNavigate = vi.fn();
    const { user } = renderWithProviders(<Sidebar onNavigate={onNavigate} />);

    await user.click(screen.getByText('Sessions'));
    expect(onNavigate).toHaveBeenCalled();
  });

  it('shows section titles even when collapsed if onNavigate is provided (mobile drawer)', () => {
    useUIStore.setState({ sidebarCollapsed: true });
    renderWithProviders(<Sidebar onNavigate={() => {}} />);
    // With onNavigate, section titles should be visible even when collapsed
    expect(screen.getByText('Core')).toBeInTheDocument();
    expect(screen.getByText('Automation')).toBeInTheDocument();
  });
});
