import { describe, it, expect, vi } from 'vitest';
import { screen } from '@testing-library/react';
import { EmptyMultiview } from '../../../components/multiview/EmptyMultiview';
import { renderWithProviders } from '../../../test/render';

const mockNavigate = vi.fn();

vi.mock('react-router', async () => {
  const actual = await vi.importActual('react-router');
  return { ...actual, useNavigate: () => mockNavigate };
});

describe('EmptyMultiview', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('renders heading and description text', () => {
    renderWithProviders(<EmptyMultiview />);
    expect(screen.getByText('Multi-View')).toBeInTheDocument();
    expect(
      screen.getByText(/View and interact with multiple terminal sessions/),
    ).toBeInTheDocument();
  });

  it('renders Go to Sessions button', () => {
    renderWithProviders(<EmptyMultiview />);
    expect(screen.getByText('Go to Sessions')).toBeInTheDocument();
  });

  it('navigates to /sessions when Go to Sessions is clicked', async () => {
    const { user } = renderWithProviders(<EmptyMultiview />);
    await user.click(screen.getByText('Go to Sessions'));
    expect(mockNavigate).toHaveBeenCalledWith('/sessions');
  });

  it('renders New Workspace button when onCreateWorkspace is provided', () => {
    const onCreate = vi.fn();
    renderWithProviders(<EmptyMultiview onCreateWorkspace={onCreate} />);
    expect(screen.getByText('New Workspace')).toBeInTheDocument();
  });

  it('does not render New Workspace button when onCreateWorkspace is not provided', () => {
    renderWithProviders(<EmptyMultiview />);
    expect(screen.queryByText('New Workspace')).not.toBeInTheDocument();
  });

  it('fires onCreateWorkspace when New Workspace button is clicked', async () => {
    const onCreate = vi.fn();
    const { user } = renderWithProviders(
      <EmptyMultiview onCreateWorkspace={onCreate} />,
    );
    await user.click(screen.getByText('New Workspace'));
    expect(onCreate).toHaveBeenCalledOnce();
  });
});
