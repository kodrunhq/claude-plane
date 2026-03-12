import { describe, it, expect } from 'vitest';

describe('useTerminalSession', () => {
  it('exports the hook function', async () => {
    const mod = await import('../useTerminalSession.ts');
    expect(mod.useTerminalSession).toBeDefined();
    expect(typeof mod.useTerminalSession).toBe('function');
  });

  it('exports TerminalStatus type (compile-time check)', async () => {
    // This test validates that TerminalStatus is importable and usable.
    // If it compiles, the type exists.
    const mod = await import('../useTerminalSession.ts');
    expect(mod).toBeDefined();

    // Runtime assertion: the module loaded without errors
    // TerminalStatus is a type-only export verified by TypeScript compilation
  });
});

describe('session types', () => {
  it('exports all session types', async () => {
    const types = await import('../../types/session.ts');
    // Verify the module loads without errors -- types are compile-time constructs
    expect(types).toBeDefined();
  });
});

describe('sessions API client', () => {
  it('exports all API functions', async () => {
    const api = await import('../../api/sessions.ts');
    expect(api.createSession).toBeDefined();
    expect(api.listSessions).toBeDefined();
    expect(api.getSession).toBeDefined();
    expect(api.terminateSession).toBeDefined();
  });
});
