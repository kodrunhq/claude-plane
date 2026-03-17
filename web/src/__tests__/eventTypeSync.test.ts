import { readFileSync } from 'node:fs';
import { resolve, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'vitest';
import { ALL_EVENT_TYPES } from '../constants/eventTypes.ts';

interface EventTypeEntry {
  name: string;
  value: string;
}

const currentDir = dirname(fileURLToPath(import.meta.url));
const jsonPath = resolve(
  currentDir,
  '../../../internal/server/event/event_types.json',
);

describe('Event type sync between Go and TypeScript', () => {
  const raw = readFileSync(jsonPath, 'utf-8');
  const backendTypes: EventTypeEntry[] = JSON.parse(raw);
  const backendValues = new Set(backendTypes.map((e) => e.value));
  const frontendValues = new Set<string>(ALL_EVENT_TYPES);

  it('frontend has every backend event type', () => {
    const missing = [...backendValues].filter((v) => !frontendValues.has(v));
    expect(
      missing,
      `Frontend is missing backend event types: ${missing.join(', ')}`,
    ).toEqual([]);
  });

  it('frontend has no extra event types beyond backend', () => {
    const extra = [...frontendValues].filter((v) => !backendValues.has(v));
    expect(
      extra,
      `Frontend has extra event types not in backend: ${extra.join(', ')}`,
    ).toEqual([]);
  });

  it('backend has exactly 18 event types', () => {
    expect(backendTypes).toHaveLength(18);
  });
});
