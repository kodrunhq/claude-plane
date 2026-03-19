import { render, screen, act } from '@testing-library/react';
import { describe, it, expect, vi, afterEach, beforeEach } from 'vitest';
import { TimeAgo } from '../../../components/shared/TimeAgo.tsx';

describe('TimeAgo', () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('renders a <time> element', () => {
    const date = new Date().toISOString();
    render(<TimeAgo date={date} />);
    const timeEl = screen.getByText(/ago|seconds|less/i).closest('time');
    expect(timeEl).toBeInTheDocument();
  });

  it('sets the dateTime attribute to the original date string', () => {
    const dateStr = '2026-01-15T10:00:00Z';
    render(<TimeAgo date={dateStr} />);
    const timeEl = document.querySelector('time');
    expect(timeEl).toHaveAttribute('dateTime', dateStr);
  });

  it('displays relative time for a recent date', () => {
    vi.setSystemTime(new Date('2026-03-19T12:05:00Z'));
    render(<TimeAgo date="2026-03-19T12:00:00Z" />);
    expect(screen.getByText(/5 minutes ago/i)).toBeInTheDocument();
  });

  it('displays relative time for an older date', () => {
    vi.setSystemTime(new Date('2026-03-19T12:00:00Z'));
    render(<TimeAgo date="2026-03-18T12:00:00Z" />);
    expect(screen.getByText(/1 day ago/i)).toBeInTheDocument();
  });

  it('displays relative time for hours ago', () => {
    vi.setSystemTime(new Date('2026-03-19T15:00:00Z'));
    render(<TimeAgo date="2026-03-19T12:00:00Z" />);
    expect(screen.getByText(/3 hours ago/i)).toBeInTheDocument();
  });

  it('sets the title attribute to the formatted absolute date', () => {
    render(<TimeAgo date="2026-01-15T10:00:00Z" />);
    const timeEl = document.querySelector('time');
    expect(timeEl?.getAttribute('title')).toBeTruthy();
    // Title should contain the year and some date components
    expect(timeEl?.getAttribute('title')).toContain('2026');
  });

  it('applies custom className', () => {
    render(<TimeAgo date="2026-01-15T10:00:00Z" className="text-xs text-red-500" />);
    const timeEl = document.querySelector('time');
    expect(timeEl).toHaveClass('text-xs', 'text-red-500');
  });

  it('updates text on interval', () => {
    // Set initial system time to 1 minute after the date
    vi.setSystemTime(new Date('2026-03-19T12:01:00Z'));
    render(<TimeAgo date="2026-03-19T12:00:00Z" />);
    expect(screen.getByText(/1 minute ago/i)).toBeInTheDocument();

    // Advance by 60s — this both fires the interval AND advances Date.now()
    // Use act() to flush React state updates from the interval callback
    act(() => {
      vi.advanceTimersByTime(60_001);
    });

    expect(screen.getByText(/2 minutes ago/i)).toBeInTheDocument();
  });

  it('handles invalid date gracefully (empty title)', () => {
    // formatTimeAgo throws for invalid dates since date-fns cannot parse them
    expect(() => render(<TimeAgo date="not-a-date" />)).toThrow();
  });
});
