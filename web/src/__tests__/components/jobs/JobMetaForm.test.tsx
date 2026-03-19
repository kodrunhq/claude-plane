import { describe, it, expect, vi } from 'vitest';
import { renderWithProviders, screen } from '../../../test/render.tsx';
import { JobMetaForm } from '../../../components/jobs/JobMetaForm.tsx';

describe('JobMetaForm', () => {
  const defaultProps = {
    name: '',
    description: '',
    onChange: vi.fn(),
  };

  function renderForm(overrides?: Partial<typeof defaultProps>) {
    const props = { ...defaultProps, ...overrides };
    return renderWithProviders(<JobMetaForm {...props} />);
  }

  it('renders Job Name label and input', () => {
    renderForm();
    expect(screen.getByLabelText('Job Name')).toBeInTheDocument();
  });

  it('renders Description label and textarea', () => {
    renderForm();
    expect(screen.getByLabelText('Description')).toBeInTheDocument();
  });

  it('displays current name value in the input', () => {
    renderForm({ name: 'Deploy Frontend' });
    expect(screen.getByLabelText('Job Name')).toHaveValue('Deploy Frontend');
  });

  it('displays current description value in the textarea', () => {
    renderForm({ description: 'Deploys the frontend to production' });
    expect(screen.getByLabelText('Description')).toHaveValue('Deploys the frontend to production');
  });

  it('shows placeholder text for name input', () => {
    renderForm();
    expect(screen.getByPlaceholderText('My Job')).toBeInTheDocument();
  });

  it('shows placeholder text for description textarea', () => {
    renderForm();
    expect(screen.getByPlaceholderText('Optional description...')).toBeInTheDocument();
  });

  it('calls onChange with ("name", value) when name input changes', async () => {
    const onChange = vi.fn();
    const { user } = renderForm({ onChange });

    const nameInput = screen.getByLabelText('Job Name');
    await user.type(nameInput, 'A');
    expect(onChange).toHaveBeenCalledWith('name', 'A');
  });

  it('calls onChange with ("description", value) when description changes', async () => {
    const onChange = vi.fn();
    const { user } = renderForm({ onChange });

    const descInput = screen.getByLabelText('Description');
    await user.type(descInput, 'B');
    expect(onChange).toHaveBeenCalledWith('description', 'B');
  });

  it('name input has type="text"', () => {
    renderForm();
    const input = screen.getByLabelText('Job Name');
    expect(input).toHaveAttribute('type', 'text');
  });

  it('description textarea has 2 rows', () => {
    renderForm();
    const textarea = screen.getByLabelText('Description');
    expect(textarea).toHaveAttribute('rows', '2');
  });

  it('name input has id="job-name"', () => {
    renderForm();
    expect(screen.getByLabelText('Job Name')).toHaveAttribute('id', 'job-name');
  });

  it('description textarea has id="job-desc"', () => {
    renderForm();
    expect(screen.getByLabelText('Description')).toHaveAttribute('id', 'job-desc');
  });
});
