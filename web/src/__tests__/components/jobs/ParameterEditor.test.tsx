import { describe, it, expect, vi } from 'vitest';
import { renderWithProviders, screen, userEvent } from '../../../test/render.tsx';
import { ParameterEditor } from '../../../components/jobs/ParameterEditor.tsx';

describe('ParameterEditor', () => {
  it('renders the Add button', () => {
    const onChange = vi.fn();
    renderWithProviders(
      <ParameterEditor parameters={{}} onChange={onChange} />,
    );

    expect(screen.getByText('Add')).toBeInTheDocument();
  });

  it('calls onChange with new parameter when Add is clicked', async () => {
    const onChange = vi.fn();
    const user = userEvent.setup();

    renderWithProviders(
      <ParameterEditor parameters={{}} onChange={onChange} />,
    );

    await user.click(screen.getByText('Add'));

    expect(onChange).toHaveBeenCalledTimes(1);
    expect(onChange).toHaveBeenCalledWith({ PARAM_1: '' });
  });

  it('increments parameter key to avoid duplicates', async () => {
    const onChange = vi.fn();
    const user = userEvent.setup();

    renderWithProviders(
      <ParameterEditor parameters={{ PARAM_1: 'val1' }} onChange={onChange} />,
    );

    await user.click(screen.getByText('Add'));

    expect(onChange).toHaveBeenCalledWith({ PARAM_1: 'val1', PARAM_2: '' });
  });

  it('renders existing parameters as rows', () => {
    const onChange = vi.fn();
    renderWithProviders(
      <ParameterEditor parameters={{ API_KEY: 'secret', MODE: 'fast' }} onChange={onChange} />,
    );

    expect(screen.getByDisplayValue('API_KEY')).toBeInTheDocument();
    expect(screen.getByDisplayValue('secret')).toBeInTheDocument();
    expect(screen.getByDisplayValue('MODE')).toBeInTheDocument();
    expect(screen.getByDisplayValue('fast')).toBeInTheDocument();
  });

  it('calls onChange when a parameter is removed', async () => {
    const onChange = vi.fn();
    const user = userEvent.setup();

    renderWithProviders(
      <ParameterEditor parameters={{ PARAM_1: 'val1', PARAM_2: 'val2' }} onChange={onChange} />,
    );

    // There should be two remove buttons (Trash2 icons)
    const removeButtons = screen.getAllByRole('button').filter(
      (btn) => btn !== screen.getByText('Add').closest('button'),
    );
    expect(removeButtons.length).toBe(2);

    // Click the first remove button
    await user.click(removeButtons[0]);

    expect(onChange).toHaveBeenCalledWith({ PARAM_2: 'val2' });
  });

  it('shows empty state message when no parameters', () => {
    const onChange = vi.fn();
    renderWithProviders(
      <ParameterEditor parameters={{}} onChange={onChange} />,
    );

    expect(screen.getByText(/No parameters defined/)).toBeInTheDocument();
  });
});
