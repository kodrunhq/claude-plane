export function envToEntries(env: Record<string, string> | undefined): ReadonlyArray<readonly [string, string]> {
  return Object.entries(env ?? {}).map(([k, v]) => [k, v] as const);
}

export function entriesToEnv(entries: ReadonlyArray<readonly [string, string]>): Record<string, string> {
  const result: Record<string, string> = {};
  for (const [k, v] of entries) {
    if (k.trim()) {
      result[k.trim()] = v;
    }
  }
  return result;
}
