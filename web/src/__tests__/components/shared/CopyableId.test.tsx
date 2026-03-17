import { render, screen } from '@testing-library/react';
import { describe, it, expect } from 'vitest';
import { CopyableId } from '../../../components/shared/CopyableId.tsx';

describe('CopyableId', () => {
  it('renders truncated ID (8 chars by default)', () => {
    render(<CopyableId id="abcdef1234567890" />);
    expect(screen.getByText('abcdef12')).toBeInTheDocument();
  });

  it('shows full ID on hover via title attribute', () => {
    render(<CopyableId id="abcdef1234567890" />);
    const wrapper = screen.getByText('abcdef12').closest('span[title]');
    expect(wrapper).toHaveAttribute('title', 'abcdef1234567890');
  });

  it('renders with custom length', () => {
    render(<CopyableId id="abcdef1234567890" length={12} />);
    expect(screen.getByText('abcdef123456')).toBeInTheDocument();
  });

  it('has a copy button', () => {
    render(<CopyableId id="abcdef1234567890" />);
    expect(screen.getByRole('button', { name: /copy/i })).toBeInTheDocument();
  });

  it('renders dash placeholder for empty ID', () => {
    render(<CopyableId id="" />);
    expect(screen.getByText('—')).toBeInTheDocument();
    expect(screen.queryByRole('button')).not.toBeInTheDocument();
  });

  it('renders full string when ID is shorter than length', () => {
    render(<CopyableId id="abc" />);
    expect(screen.getByText('abc')).toBeInTheDocument();
  });
});
