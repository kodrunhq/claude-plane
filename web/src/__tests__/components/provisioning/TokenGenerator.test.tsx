import { describe, it, expect, vi, beforeEach } from 'vitest';
import { screen } from '@testing-library/react';
import { TokenGenerator } from '../../../components/provisioning/TokenGenerator';
import { renderWithProviders } from '../../../test/render';

const mockMutateAsync = vi.fn();
const mockCreateToken = {
  mutateAsync: mockMutateAsync,
  isPending: false,
};

vi.mock('../../../hooks/useProvisioning', () => ({
  useCreateProvisioningToken: () => mockCreateToken,
}));

vi.mock('../../../lib/clipboard', () => ({
  copyToClipboard: vi.fn().mockResolvedValue(undefined),
}));

vi.mock('sonner', () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

describe('TokenGenerator', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockCreateToken.isPending = false;
  });

  it('renders the heading and description', () => {
    renderWithProviders(<TokenGenerator />);
    expect(screen.getByText('Generate Provisioning Token')).toBeInTheDocument();
    expect(
      screen.getByText('Creates a one-time install token valid for 1 hour'),
    ).toBeInTheDocument();
  });

  it('renders Machine ID input with required indicator', () => {
    renderWithProviders(<TokenGenerator />);
    expect(screen.getByPlaceholderText('e.g. worker-01')).toBeInTheDocument();
    expect(screen.getByText('*')).toBeInTheDocument();
  });

  it('renders OS select with linux and darwin options', () => {
    renderWithProviders(<TokenGenerator />);
    const selects = screen.getAllByRole('combobox');
    const osSelect = selects.find((el) => el.closest('div')?.querySelector('label')?.textContent === 'OS')! as HTMLSelectElement;
    const options = Array.from(osSelect.options).map((o) => o.value);
    expect(options).toEqual(['linux', 'darwin']);
    expect(osSelect.value).toBe('linux');
  });

  it('renders Architecture select with amd64 and arm64 options', () => {
    renderWithProviders(<TokenGenerator />);
    const selects = screen.getAllByRole('combobox');
    const archSelect = selects.find((el) => el.closest('div')?.querySelector('label')?.textContent === 'Architecture')! as HTMLSelectElement;
    const options = Array.from(archSelect.options).map((o) => o.value);
    expect(options).toEqual(['amd64', 'arm64']);
    expect(archSelect.value).toBe('amd64');
  });

  it('renders Generate Token button', () => {
    renderWithProviders(<TokenGenerator />);
    expect(screen.getByText('Generate Token')).toBeInTheDocument();
  });

  it('disables Generate Token button when machine ID is empty', () => {
    renderWithProviders(<TokenGenerator />);
    expect(screen.getByText('Generate Token')).toBeDisabled();
  });

  it('enables Generate Token button when machine ID is provided', async () => {
    const { user } = renderWithProviders(<TokenGenerator />);
    await user.type(screen.getByPlaceholderText('e.g. worker-01'), 'worker-1');
    expect(screen.getByText('Generate Token')).not.toBeDisabled();
  });

  it('shows Generating... text when mutation is pending', () => {
    mockCreateToken.isPending = true;
    renderWithProviders(<TokenGenerator />);
    expect(screen.getByText('Generating...')).toBeInTheDocument();
  });

  it('calls mutateAsync with form params when submitted', async () => {
    mockMutateAsync.mockResolvedValue({
      token: 'tok-123',
      short_code: 'ABC123',
      expires_at: '2026-01-15T11:00:00Z',
      curl_command: 'curl https://...',
      join_command: 'claude-plane-agent join ABC123',
    });
    const { user } = renderWithProviders(<TokenGenerator />);
    await user.type(screen.getByPlaceholderText('e.g. worker-01'), 'my-machine');
    await user.click(screen.getByText('Generate Token'));
    expect(mockMutateAsync).toHaveBeenCalledWith({
      machine_id: 'my-machine',
      os: 'linux',
      arch: 'amd64',
    });
  });

  it('displays result with short code after successful generation', async () => {
    mockMutateAsync.mockResolvedValue({
      token: 'tok-123',
      short_code: 'XYZ789',
      expires_at: new Date(Date.now() + 3600000).toISOString(),
      curl_command: 'curl https://example.com/install.sh',
      join_command: 'claude-plane-agent join XYZ789',
    });
    const { user } = renderWithProviders(<TokenGenerator />);
    await user.type(screen.getByPlaceholderText('e.g. worker-01'), 'worker-x');
    await user.click(screen.getByText('Generate Token'));
    expect(screen.getByText('XYZ789')).toBeInTheDocument();
    expect(screen.getByText('Join Code')).toBeInTheDocument();
  });

  it('displays join command after successful generation', async () => {
    mockMutateAsync.mockResolvedValue({
      token: 'tok-123',
      short_code: 'ABC',
      expires_at: new Date(Date.now() + 3600000).toISOString(),
      curl_command: 'curl https://example.com',
      join_command: 'claude-plane-agent join ABC',
    });
    const { user } = renderWithProviders(<TokenGenerator />);
    await user.type(screen.getByPlaceholderText('e.g. worker-01'), 'worker-1');
    await user.click(screen.getByText('Generate Token'));
    expect(screen.getByText('claude-plane-agent join ABC')).toBeInTheDocument();
    expect(screen.getByText('Run on the target machine:')).toBeInTheDocument();
  });

  it('renders Copy buttons for short code and join command', async () => {
    mockMutateAsync.mockResolvedValue({
      token: 'tok-123',
      short_code: 'CODE1',
      expires_at: new Date(Date.now() + 3600000).toISOString(),
      curl_command: 'curl ...',
      join_command: 'join CODE1',
    });
    const { user } = renderWithProviders(<TokenGenerator />);
    await user.type(screen.getByPlaceholderText('e.g. worker-01'), 'w1');
    await user.click(screen.getByText('Generate Token'));
    // There should be multiple Copy buttons
    const copyButtons = screen.getAllByText('Copy');
    expect(copyButtons.length).toBeGreaterThanOrEqual(2);
  });

  it('shows Advanced section with curl command when toggled', async () => {
    mockMutateAsync.mockResolvedValue({
      token: 'tok-123',
      short_code: 'CODE2',
      expires_at: new Date(Date.now() + 3600000).toISOString(),
      curl_command: 'curl -fsSL https://example.com/install.sh | bash',
      join_command: 'join CODE2',
    });
    const { user } = renderWithProviders(<TokenGenerator />);
    await user.type(screen.getByPlaceholderText('e.g. worker-01'), 'w2');
    await user.click(screen.getByText('Generate Token'));
    // Advanced section should be hidden by default
    expect(screen.queryByText('For scripted provisioning:')).not.toBeInTheDocument();
    // Click to expand
    await user.click(screen.getByText('Advanced (curl command)'));
    expect(screen.getByText('For scripted provisioning:')).toBeInTheDocument();
    expect(
      screen.getByText('curl -fsSL https://example.com/install.sh | bash'),
    ).toBeInTheDocument();
  });

  it('resets form after successful generation', async () => {
    mockMutateAsync.mockResolvedValue({
      token: 'tok-123',
      short_code: 'RST',
      expires_at: new Date(Date.now() + 3600000).toISOString(),
      curl_command: 'curl ...',
      join_command: 'join RST',
    });
    const { user } = renderWithProviders(<TokenGenerator />);
    await user.type(screen.getByPlaceholderText('e.g. worker-01'), 'machine-99');
    await user.click(screen.getByText('Generate Token'));
    // Machine ID input should be cleared
    expect(screen.getByPlaceholderText('e.g. worker-01')).toHaveValue('');
  });

  it('allows changing OS and architecture', async () => {
    mockMutateAsync.mockResolvedValue({
      token: 'tok',
      short_code: 'OS',
      expires_at: new Date(Date.now() + 3600000).toISOString(),
      curl_command: 'curl ...',
      join_command: 'join OS',
    });
    const { user } = renderWithProviders(<TokenGenerator />);
    const selects = screen.getAllByRole('combobox');
    const osSelect = selects.find((el) => el.closest('div')?.querySelector('label')?.textContent === 'OS')!;
    const archSelect = selects.find((el) => el.closest('div')?.querySelector('label')?.textContent === 'Architecture')!;
    await user.selectOptions(osSelect, 'darwin');
    await user.selectOptions(archSelect, 'arm64');
    await user.type(screen.getByPlaceholderText('e.g. worker-01'), 'mac-m1');
    await user.click(screen.getByText('Generate Token'));
    expect(mockMutateAsync).toHaveBeenCalledWith({
      machine_id: 'mac-m1',
      os: 'darwin',
      arch: 'arm64',
    });
  });

  it('does not show result section before generation', () => {
    renderWithProviders(<TokenGenerator />);
    expect(screen.queryByText('Join Code')).not.toBeInTheDocument();
  });
});
