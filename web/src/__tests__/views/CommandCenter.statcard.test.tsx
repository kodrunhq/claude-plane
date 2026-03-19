import { describe, it, expect } from 'vitest';
import { waitFor } from '@testing-library/react';
import { CommandCenter } from '../../views/CommandCenter';
import { renderWithProviders } from '../../test/render';

function waitForStatCards() {
  return waitFor(() => {
    const cards = document.querySelectorAll('.stat-card');
    expect(cards.length).toBe(5);
  });
}

describe('CommandCenter StatCard regression tests', () => {
  // ── Bug regression: stat card text overflowed its flex container ───────────
  it('StatCard inner text wrapper has min-w-0 class to prevent flex overflow', async () => {
    renderWithProviders(<CommandCenter />);
    await waitForStatCards();

    const statCards = document.querySelectorAll('.stat-card');
    for (const card of statCards) {
      const textWrapper = card.querySelector('.min-w-0');
      expect(textWrapper).not.toBeNull();
    }
  });

  it('StatCard value text has truncate class to handle overflow', async () => {
    renderWithProviders(<CommandCenter />);
    await waitForStatCards();

    const statCards = document.querySelectorAll('.stat-card');
    for (const card of statCards) {
      const valueEl = card.querySelector('.text-2xl');
      expect(valueEl).not.toBeNull();
      expect(valueEl!.classList.contains('truncate')).toBe(true);
    }
  });

  it('StatCard label text has truncate class to handle overflow', async () => {
    renderWithProviders(<CommandCenter />);
    await waitForStatCards();

    const statCards = document.querySelectorAll('.stat-card');
    for (const card of statCards) {
      const labelEl = card.querySelector('.text-xs.text-text-secondary');
      expect(labelEl).not.toBeNull();
      expect(labelEl!.classList.contains('truncate')).toBe(true);
    }
  });

  it('renders all 5 stat cards with correct labels', async () => {
    renderWithProviders(<CommandCenter />);
    await waitForStatCards();

    const statCards = document.querySelectorAll('.stat-card');
    const labels = Array.from(statCards).map(
      (card) => card.querySelector('.text-xs.text-text-secondary')?.textContent,
    );
    expect(labels).toContain('Active Sessions');
    expect(labels).toContain('Online Machines');
    expect(labels).toContain('Total Jobs');
    expect(labels).toContain('Recent Runs');
    expect(labels).toContain('Completion Rate');
  });
});
