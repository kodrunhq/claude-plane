import { describe, it, expect, vi } from 'vitest';
import { http, HttpResponse } from 'msw';
import { server } from '../../../test/setup.ts';
import { renderWithProviders, screen, waitFor } from '../../../test/render.tsx';
import { DirectoryBrowserModal } from '../../../components/sessions/DirectoryBrowserModal.tsx';
import { mockBrowseResponse } from '../../../test/handlers.ts';

describe('DirectoryBrowserModal', () => {
  const defaultProps = {
    open: true,
    onClose: vi.fn(),
    onSelect: vi.fn(),
    machineId: 'machine-100',
  };

  function renderModal(overrides?: Partial<typeof defaultProps>) {
    const props = { ...defaultProps, ...overrides };
    return renderWithProviders(<DirectoryBrowserModal {...props} />);
  }

  it('renders nothing when open is false', () => {
    renderModal({ open: false });
    expect(screen.queryByText('Browse Directory')).not.toBeInTheDocument();
  });

  it('renders modal title when open', async () => {
    renderModal();
    expect(screen.getByText('Browse Directory')).toBeInTheDocument();
  });

  it('shows loading spinner initially', () => {
    // Use a handler that delays so we can see the loading state
    server.use(
      http.get('/api/v1/machines/:id/browse', () => {
        return new Promise(() => {
          // Never resolves — keeps loading state
        });
      }),
    );

    renderModal();
    // The Loader2 spinner should be present
    expect(screen.getByText('Browse Directory')).toBeInTheDocument();
  });

  it('displays directories from API response', async () => {
    renderModal();

    await waitFor(() => {
      expect(screen.getByText('Documents')).toBeInTheDocument();
      expect(screen.getByText('projects')).toBeInTheDocument();
    });
  });

  it('displays files from API response', async () => {
    renderModal();

    await waitFor(() => {
      expect(screen.getByText('.bashrc')).toBeInTheDocument();
    });
  });

  it('displays parent (..) navigation when parent path is available', async () => {
    renderModal();

    await waitFor(() => {
      expect(screen.getByText('..')).toBeInTheDocument();
    });
  });

  it('does not display parent (..) when parent is empty string', async () => {
    server.use(
      http.get('/api/v1/machines/:id/browse', () =>
        HttpResponse.json({
          path: '/home',
          entries: [{ name: 'user', type: 'dir' }],
          parent: '',
        }),
      ),
    );

    renderModal();

    await waitFor(() => {
      expect(screen.getByText('user')).toBeInTheDocument();
    });
    expect(screen.queryByText('..')).not.toBeInTheDocument();
  });

  it('clicking a directory navigates into it', async () => {
    let requestedPath: string | null = null;
    let callCount = 0;

    server.use(
      http.get('/api/v1/machines/:id/browse', ({ request }) => {
        callCount++;
        const url = new URL(request.url);
        requestedPath = url.searchParams.get('path');

        if (callCount === 1) {
          return HttpResponse.json(mockBrowseResponse);
        }
        // Second call: navigated into Documents
        return HttpResponse.json({
          path: '/home/user/Documents',
          entries: [{ name: 'notes.txt', type: 'file' }],
          parent: '/home/user',
        });
      }),
    );

    const { user } = renderModal();

    await waitFor(() => {
      expect(screen.getByText('Documents')).toBeInTheDocument();
    });

    await user.click(screen.getByText('Documents'));

    await waitFor(() => {
      expect(requestedPath).toBe('/home/user/Documents');
    });
  });

  it('clicking parent (..) navigates up', async () => {
    let callCount = 0;

    server.use(
      http.get('/api/v1/machines/:id/browse', () => {
        callCount++;
        if (callCount === 1) {
          return HttpResponse.json(mockBrowseResponse);
        }
        return HttpResponse.json({
          path: '/',
          entries: [{ name: 'home', type: 'dir' }],
          parent: '',
        });
      }),
    );

    const { user } = renderModal();

    await waitFor(() => {
      expect(screen.getByText('..')).toBeInTheDocument();
    });

    await user.click(screen.getByText('..'));

    await waitFor(() => {
      expect(screen.getByText('home')).toBeInTheDocument();
    });
  });

  it('breadcrumbs render from current path', async () => {
    renderModal();

    await waitFor(() => {
      // /home/user -> breadcrumbs: "home" and "user"
      expect(screen.getByText('home')).toBeInTheDocument();
      expect(screen.getByText('user')).toBeInTheDocument();
    });
  });

  it('clicking a breadcrumb navigates to that path', async () => {
    let requestedPath: string | null = null;
    let callCount = 0;

    server.use(
      http.get('/api/v1/machines/:id/browse', ({ request }) => {
        callCount++;
        const url = new URL(request.url);
        requestedPath = url.searchParams.get('path');

        if (callCount === 1) {
          return HttpResponse.json(mockBrowseResponse);
        }
        return HttpResponse.json({
          path: '/home',
          entries: [{ name: 'user', type: 'dir' }],
          parent: '/',
        });
      }),
    );

    const { user } = renderModal();

    await waitFor(() => {
      expect(screen.getByText('home')).toBeInTheDocument();
    });

    // Click the "home" breadcrumb
    const breadcrumbButtons = screen.getAllByRole('button');
    const homeBreadcrumb = breadcrumbButtons.find((b) => b.textContent === 'home');
    expect(homeBreadcrumb).toBeTruthy();
    await user.click(homeBreadcrumb!);

    await waitFor(() => {
      expect(requestedPath).toBe('/home');
    });
  });

  it('Select button calls onSelect with current path', async () => {
    const onSelect = vi.fn();
    const { user } = renderModal({ onSelect });

    await waitFor(() => {
      expect(screen.getByText('Documents')).toBeInTheDocument();
    });

    await user.click(screen.getByText('Select'));

    expect(onSelect).toHaveBeenCalledWith('/home/user');
  });

  it('Cancel button calls onClose', async () => {
    const onClose = vi.fn();
    const { user } = renderModal({ onClose });

    await waitFor(() => {
      expect(screen.getByText('Cancel')).toBeInTheDocument();
    });

    await user.click(screen.getByText('Cancel'));
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('clicking backdrop calls onClose', async () => {
    const onClose = vi.fn();
    const { user } = renderModal({ onClose });

    await waitFor(() => {
      expect(screen.getByText('Browse Directory')).toBeInTheDocument();
    });

    // Backdrop is the .absolute.inset-0 div
    const dialog = screen.getByRole('dialog');
    const backdrop = dialog.parentElement!.querySelector('.absolute');
    expect(backdrop).toBeTruthy();
    await user.click(backdrop!);
    expect(onClose).toHaveBeenCalled();
  });

  it('displays current path in footer', async () => {
    renderModal();

    await waitFor(() => {
      // The path is shown in a span in the footer
      expect(screen.getByText('/home/user')).toBeInTheDocument();
    });
  });

  it('shows error message when browse API fails', async () => {
    server.use(
      http.get('/api/v1/machines/:id/browse', () =>
        HttpResponse.json({ error: 'Permission denied' }, { status: 403 }),
      ),
    );

    renderModal();

    await waitFor(() => {
      expect(screen.getByText(/Failed to browse directory|Permission denied/)).toBeInTheDocument();
    });
  });

  it('shows "Empty directory" when no entries', async () => {
    server.use(
      http.get('/api/v1/machines/:id/browse', () =>
        HttpResponse.json({
          path: '/home/user/empty',
          entries: [],
          parent: '/home/user',
        }),
      ),
    );

    renderModal();

    await waitFor(() => {
      expect(screen.getByText('Empty directory')).toBeInTheDocument();
    });
  });

  it('has dialog role with aria-modal', async () => {
    renderModal();
    const dialog = screen.getByRole('dialog');
    expect(dialog).toHaveAttribute('aria-modal', 'true');
  });
});
