import { render, screen } from '@testing-library/react';
import { describe, it, expect } from 'vitest';
import { RunStatusBadge } from '../../../components/runs/RunStatusBadge.tsx';

describe('RunStatusBadge', () => {
  const knownStatuses = ['pending', 'running', 'completed', 'failed', 'cancelled'];

  describe('renders all known status variants', () => {
    for (const status of knownStatuses) {
      it(`renders "${status}" text`, () => {
        render(<RunStatusBadge status={status} />);
        expect(screen.getByText(status)).toBeInTheDocument();
      });
    }
  });

  describe('applies correct color classes per status', () => {
    it('applies gray colors for pending', () => {
      const { container } = render(<RunStatusBadge status="pending" />);
      const badge = container.querySelector('span');
      expect(badge).toHaveClass('bg-gray-500/20', 'text-gray-400');
    });

    it('applies blue colors for running', () => {
      const { container } = render(<RunStatusBadge status="running" />);
      const badge = container.querySelector('span');
      expect(badge).toHaveClass('bg-blue-500/20', 'text-blue-400');
    });

    it('applies green colors for completed', () => {
      const { container } = render(<RunStatusBadge status="completed" />);
      const badge = container.querySelector('span');
      expect(badge).toHaveClass('bg-green-500/20', 'text-green-400');
    });

    it('applies red colors for failed', () => {
      const { container } = render(<RunStatusBadge status="failed" />);
      const badge = container.querySelector('span');
      expect(badge).toHaveClass('bg-red-500/20', 'text-red-400');
    });

    it('applies yellow colors for cancelled', () => {
      const { container } = render(<RunStatusBadge status="cancelled" />);
      const badge = container.querySelector('span');
      expect(badge).toHaveClass('bg-yellow-500/20', 'text-yellow-400');
    });
  });

  describe('unknown status falls back to pending colors', () => {
    it('uses gray (pending) colors for unknown status', () => {
      const { container } = render(<RunStatusBadge status="unknown_state" />);
      const badge = container.querySelector('span');
      expect(badge).toHaveClass('bg-gray-500/20', 'text-gray-400');
      expect(screen.getByText('unknown_state')).toBeInTheDocument();
    });
  });

  describe('size variants', () => {
    it('defaults to md size', () => {
      const { container } = render(<RunStatusBadge status="running" />);
      const badge = container.querySelector('span');
      expect(badge).toHaveClass('px-2', 'text-xs');
    });

    it('applies sm size classes', () => {
      const { container } = render(<RunStatusBadge status="running" size="sm" />);
      const badge = container.querySelector('span');
      expect(badge).toHaveClass('px-1.5', 'text-[10px]');
    });
  });

  it('renders as a span with rounded-full class', () => {
    const { container } = render(<RunStatusBadge status="completed" />);
    const badge = container.querySelector('span');
    expect(badge).toHaveClass('rounded-full');
  });
});
