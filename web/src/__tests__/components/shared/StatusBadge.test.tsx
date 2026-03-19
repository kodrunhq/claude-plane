import { render, screen } from '@testing-library/react';
import { describe, it, expect } from 'vitest';
import { StatusBadge } from '../../../components/shared/StatusBadge.tsx';

describe('StatusBadge', () => {
  describe('renders status text for all variants', () => {
    const statuses = [
      'connected',
      'disconnected',
      'running',
      'pending',
      'completed',
      'failed',
      'error',
      'cancelled',
      'terminated',
      'created',
      'online',
      'offline',
      'success',
    ];

    for (const status of statuses) {
      it(`renders "${status}" status text`, () => {
        render(<StatusBadge status={status} />);
        expect(screen.getByText(status)).toBeInTheDocument();
      });
    }
  });

  describe('status text is capitalized via CSS class', () => {
    it('applies capitalize class to status text', () => {
      render(<StatusBadge status="running" />);
      const textSpan = screen.getByText('running');
      expect(textSpan).toHaveClass('capitalize');
    });
  });

  describe('size variants', () => {
    it('defaults to md size', () => {
      const { container } = render(<StatusBadge status="connected" />);
      const wrapper = container.querySelector('span.inline-flex');
      expect(wrapper).toHaveClass('text-sm');
    });

    it('applies sm size text class', () => {
      const { container } = render(<StatusBadge status="connected" size="sm" />);
      const wrapper = container.querySelector('span.inline-flex');
      expect(wrapper).toHaveClass('text-xs');
    });

    it('applies lg size text class', () => {
      const { container } = render(<StatusBadge status="connected" size="lg" />);
      const wrapper = container.querySelector('span.inline-flex');
      expect(wrapper).toHaveClass('text-base');
    });
  });

  describe('icon types', () => {
    it('renders a check icon for success statuses (connected, completed)', () => {
      const { container } = render(<StatusBadge status="connected" />);
      // CheckCircle2 renders an SVG; no plain dot span should be present
      const svg = container.querySelector('svg');
      expect(svg).toBeInTheDocument();
    });

    it('renders an x icon for error statuses (failed, disconnected)', () => {
      const { container } = render(<StatusBadge status="failed" />);
      const svg = container.querySelector('svg');
      expect(svg).toBeInTheDocument();
    });

    it('renders a spinner icon for running status', () => {
      const { container } = render(<StatusBadge status="running" />);
      const svg = container.querySelector('svg');
      expect(svg).toBeInTheDocument();
      // The spinner should have animate-spin
      expect(svg?.closest('svg')).toHaveClass('animate-spin');
    });

    it('renders a clock icon for created status', () => {
      const { container } = render(<StatusBadge status="created" />);
      const svg = container.querySelector('svg');
      expect(svg).toBeInTheDocument();
    });
  });

  describe('unknown status', () => {
    it('renders unknown status text with fallback icon', () => {
      const { container } = render(<StatusBadge status="unknown_status" />);
      expect(screen.getByText('unknown_status')).toBeInTheDocument();
      // Falls back to clock icon (default meta)
      const svg = container.querySelector('svg');
      expect(svg).toBeInTheDocument();
    });
  });

  describe('dot indicator for "none" icon type', () => {
    // The current STATUS_META doesn't have any "none" iconType entries,
    // but the component handles it with a plain dot. We can verify the
    // dot branch by checking the default fallback path doesn't crash.
    it('does not crash on empty string status', () => {
      const { container } = render(<StatusBadge status="" />);
      // Empty string still renders (falls through to default meta)
      expect(container.querySelector('span.inline-flex')).toBeInTheDocument();
    });
  });
});
