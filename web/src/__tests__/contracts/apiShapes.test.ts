/**
 * API shape contract tests.
 *
 * These verify that mock data used in tests contains the same required fields
 * that the TypeScript types declare as non-optional. When the backend changes
 * a response shape and mocks are updated to match, these tests catch any field
 * that the frontend types still expect but the mock no longer provides.
 */

import { describe, it, expect } from 'vitest';
import {
  mockSessions,
  mockMachines,
  mockJobs,
  mockRuns,
  mockTemplates,
} from '../../test/handlers.ts';

/** Assert that every item in the array has all listed keys with non-nullish values. */
function expectRequiredFields(
  items: readonly object[],
  requiredFields: string[],
  label: string,
) {
  expect(items.length).toBeGreaterThan(0);
  for (const [i, item] of items.entries()) {
    for (const field of requiredFields) {
      expect(item).toHaveProperty(field);
      const val = (item as Record<string, unknown>)[field];
      expect(val, `${label}[${i}].${field} should not be nullish`).not.toBeNull();
      expect(val, `${label}[${i}].${field} should not be undefined`).not.toBeUndefined();
    }
  }
}

/** Assert that no item has any of the listed keys with a truthy value. */
function expectAbsentFields(
  items: readonly object[],
  absentFields: string[],
  label: string,
) {
  for (const [i, item] of items.entries()) {
    const record = item as Record<string, unknown>;
    for (const field of absentFields) {
      const val = record[field];
      if (val !== undefined && val !== null && val !== '') {
        throw new Error(
          `${label}[${i}].${field} should be absent or empty, got: ${JSON.stringify(val)}`,
        );
      }
    }
  }
}

describe('API response shape contracts', () => {
  it('machines mock matches required Machine fields', () => {
    expectRequiredFields(
      mockMachines,
      ['machine_id', 'display_name', 'status', 'max_sessions', 'created_at'],
      'machines',
    );
  });

  it('sessions mock matches required Session fields', () => {
    expectRequiredFields(
      mockSessions,
      ['session_id', 'machine_id', 'status', 'command', 'created_at'],
      'sessions',
    );
  });

  it('jobs mock matches required Job fields', () => {
    expectRequiredFields(mockJobs, ['job_id', 'name', 'created_at'], 'jobs');
  });

  it('runs mock matches required Run fields', () => {
    expectRequiredFields(mockRuns, ['run_id', 'job_id', 'status', 'created_at'], 'runs');
  });

  it('templates mock matches required SessionTemplate fields', () => {
    expectRequiredFields(
      mockTemplates,
      ['template_id', 'user_id', 'name', 'terminal_rows', 'terminal_cols', 'created_at'],
      'templates',
    );
  });

  it('session list response should not expose sensitive fields', () => {
    // In the real backend, list responses strip env_vars, args, and initial_prompt.
    // Mocks may include them as optional/undefined — verify they are not present with values.
    expectAbsentFields(mockSessions, ['env_vars', 'args', 'initial_prompt'], 'sessions');
  });
});
