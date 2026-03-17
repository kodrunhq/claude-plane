import fs from 'node:fs';
import path from 'node:path';
import { describe, expect, it } from 'vitest';
import { ALL_EVENT_TYPES } from '../constants/eventTypes';

interface EventTypeEntry {
  name: string;
  value: string;
}

const jsonPath = path.resolve(
  __dirname,
  '../../../internal/server/event/event_types.json',
);

describe('Event type sync between Go and TypeScript', () => {
  const raw = fs.readFileSync(jsonPath, 'utf-8');
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
