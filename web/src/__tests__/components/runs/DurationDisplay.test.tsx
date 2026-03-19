import { render, screen, act } from '@testing-library/react';
import { describe, it, expect, vi, afterEach, beforeEach } from 'vitest';
import { DurationDisplay } from '../../../components/runs/DurationDisplay.tsx';

describe('DurationDisplay', () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  describe('no start time', () => {
    it('renders a dash when startedAt is undefined', () => {
      const { container } = render(<DurationDisplay />);
      // The mdash entity renders as \u2014
      expect(container.querySelector('span')).toHaveTextContent('\u2014');
    });
  });

  describe('completed runs (startedAt + completedAt)', () => {
    it('formats seconds-only duration', () => {
      render(
        <DurationDisplay
          startedAt="2026-01-15T10:00:00Z"
          completedAt="2026-01-15T10:00:30Z"
        />,
      );
      expect(screen.getByText('30s')).toBeInTheDocument();
    });

    it('formats minutes and seconds', () => {
      render(
        <DurationDisplay
          startedAt="2026-01-15T10:00:00Z"
          completedAt="2026-01-15T10:05:12Z"
        />,
      );
      expect(screen.getByText('5m 12s')).toBeInTheDocument();
    });

    it('formats hours and minutes', () => {
      render(
        <DurationDisplay
          startedAt="2026-01-15T10:00:00Z"
          completedAt="2026-01-15T11:23:00Z"
        />,
      );
      expect(screen.getByText('1h 23m')).toBeInTheDocument();
    });

    it('formats 0 seconds for same start and end', () => {
      render(
        <DurationDisplay
          startedAt="2026-01-15T10:00:00Z"
          completedAt="2026-01-15T10:00:00Z"
        />,
      );
      expect(screen.getByText('0s')).toBeInTheDocument();
    });

    it('formats multi-hour duration', () => {
      render(
        <DurationDisplay
          startedAt="2026-01-15T10:00:00Z"
          completedAt="2026-01-15T13:45:00Z"
        />,
      );
      expect(screen.getByText('3h 45m')).toBeInTheDocument();
    });
  });

  describe('in-progress runs (startedAt only, no completedAt)', () => {
    it('calculates elapsed time from now', () => {
      vi.setSystemTime(new Date('2026-01-15T10:02:30Z'));

      render(<DurationDisplay startedAt="2026-01-15T10:00:00Z" />);
      expect(screen.getByText('2m 30s')).toBeInTheDocument();
    });

    it('updates elapsed time every second', () => {
      vi.setSystemTime(new Date('2026-01-15T10:00:05Z'));

      render(<DurationDisplay startedAt="2026-01-15T10:00:00Z" />);
      expect(screen.getByText('5s')).toBeInTheDocument();

      // Advance by 10s — this both fires the 1s interval AND advances Date.now()
      // Use act() to flush React state updates from the interval callback
      act(() => {
        vi.advanceTimersByTime(10_001);
      });

      expect(screen.getByText('15s')).toBeInTheDocument();
    });
  });

  describe('prefix option', () => {
    it('prepends prefix to the formatted duration', () => {
      render(
        <DurationDisplay
          startedAt="2026-01-15T10:00:00Z"
          completedAt="2026-01-15T10:01:00Z"
          prefix="Took"
        />,
      );
      expect(screen.getByText('Took 1m 0s')).toBeInTheDocument();
    });

    it('does not show prefix when there is no duration (no startedAt)', () => {
      const { container } = render(<DurationDisplay prefix="Took" />);
      expect(container.textContent).not.toContain('Took');
    });
  });

  describe('edge cases', () => {
    it('handles transition from in-progress to completed on re-render', () => {
      vi.setSystemTime(new Date('2026-01-15T10:00:10Z'));

      const { rerender } = render(
        <DurationDisplay startedAt="2026-01-15T10:00:00Z" />,
      );
      expect(screen.getByText('10s')).toBeInTheDocument();

      rerender(
        <DurationDisplay
          startedAt="2026-01-15T10:00:00Z"
          completedAt="2026-01-15T10:00:45Z"
        />,
      );
      expect(screen.getByText('45s')).toBeInTheDocument();
    });

    it('resets to dash when startedAt is removed', () => {
      const { rerender, container } = render(
        <DurationDisplay startedAt="2026-01-15T10:00:00Z" completedAt="2026-01-15T10:00:30Z" />,
      );
      expect(screen.getByText('30s')).toBeInTheDocument();

      rerender(<DurationDisplay />);
      expect(container.querySelector('span')).toHaveTextContent('\u2014');
    });
  });
});
