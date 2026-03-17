import { describe, it, expect, vi } from 'vitest';
import { renderWithProviders, screen } from '../../../test/render.tsx';
import { TemplateForm } from '../../../components/templates/TemplateForm.tsx';

describe('TemplateForm', () => {
  it('does not render terminal rows/cols inputs', () => {
    renderWithProviders(
      <TemplateForm onSubmit={vi.fn()} onCancel={vi.fn()} />,
    );

    expect(screen.queryByLabelText(/terminal rows/i)).not.toBeInTheDocument();
    expect(screen.queryByLabelText(/terminal cols/i)).not.toBeInTheDocument();
  });

  it('does not render tags input', () => {
    renderWithProviders(
      <TemplateForm onSubmit={vi.fn()} onCancel={vi.fn()} />,
    );

    expect(screen.queryByPlaceholderText(/press enter to add a tag/i)).not.toBeInTheDocument();
    expect(screen.queryByText('Tags')).not.toBeInTheDocument();
  });

  it('renders core form fields', () => {
    renderWithProviders(
      <TemplateForm onSubmit={vi.fn()} onCancel={vi.fn()} />,
    );

    expect(screen.getByPlaceholderText('My Template')).toBeInTheDocument();
    expect(screen.getByPlaceholderText('claude')).toBeInTheDocument();
    expect(screen.getByPlaceholderText('/home/user/project')).toBeInTheDocument();
    expect(screen.getByText('Create Template')).toBeInTheDocument();
  });

  it('shows Update Template button when editing', () => {
    const initialValues = {
      template_id: 'tmpl-1',
      user_id: 'user-1',
      name: 'Test',
      terminal_rows: 24,
      terminal_cols: 80,
      timeout_seconds: 0,
      created_at: '2026-01-01T00:00:00Z',
      updated_at: '2026-01-01T00:00:00Z',
    };

    renderWithProviders(
      <TemplateForm
        initialValues={initialValues}
        onSubmit={vi.fn()}
        onCancel={vi.fn()}
      />,
    );

    expect(screen.getByText('Update Template')).toBeInTheDocument();
  });
});
