import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { TerminalView } from '../../../components/terminal/TerminalView.tsx';

// Mock useTerminalSession to control status
const mockFocusTerminal = vi.fn();
let mockStatus = 'connecting';
let mockShowScrollButton = false;

vi.mock('../../../hooks/useTerminalSession.ts', () => ({
  useTerminalSession: () => ({
    status: mockStatus,
    term: { current: null },
    ws: { current: null },
    fitTerminal: vi.fn(),
    focusTerminal: mockFocusTerminal,
    showScrollButton: mockShowScrollButton,
    scrollToBottom: vi.fn(),
  }),
}));

function renderTerminalView(sessionId = 'test-session-id') {
  return render(<TerminalView sessionId={sessionId} />);
}

describe('TerminalView', () => {
  it('renders connecting status', () => {
    mockStatus = 'connecting';
    renderTerminalView();
    expect(screen.getByText('Connecting...')).toBeInTheDocument();
  });

  it('renders replaying status', () => {
    mockStatus = 'replaying';
    renderTerminalView();
    expect(screen.getByText('Loading history...')).toBeInTheDocument();
  });

  it('renders live status', () => {
    mockStatus = 'live';
    renderTerminalView();
    expect(screen.getByText('Connected')).toBeInTheDocument();
  });

  it('renders ended status', () => {
    mockStatus = 'ended';
    renderTerminalView();
    expect(screen.getByText('Session ended')).toBeInTheDocument();
  });

  it('renders disconnected status', () => {
    mockStatus = 'disconnected';
    renderTerminalView();
    expect(screen.getByText('Disconnected')).toBeInTheDocument();
  });

  it('renders agent_offline status with descriptive label', () => {
    mockStatus = 'agent_offline';
    renderTerminalView();
    expect(screen.getByText('Agent offline')).toBeInTheDocument();
  });

  it('shows agent offline overlay with explanation message', () => {
    mockStatus = 'agent_offline';
    renderTerminalView();
    expect(
      screen.getByText(/agent that ran this session is offline/i),
    ).toBeInTheDocument();
  });

  it('does not show agent offline overlay for other statuses', () => {
    mockStatus = 'ended';
    renderTerminalView();
    expect(
      screen.queryByText(/agent that ran this session is offline/i),
    ).not.toBeInTheDocument();
  });

  it('shows session ID prefix in status bar', () => {
    mockStatus = 'live';
    renderTerminalView('abcdef12-1234-1234-1234-123456789abc');
    expect(screen.getByText('abcdef12')).toBeInTheDocument();
  });

  it('renders scroll-to-bottom button when showScrollButton is true', () => {
    mockStatus = 'live';
    mockShowScrollButton = true;
    renderTerminalView();
    expect(screen.getByText('Bottom')).toBeInTheDocument();
    mockShowScrollButton = false;
  });
});
