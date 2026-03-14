/**
 * Extracts unique variable names from a template prompt.
 * Variables use ${VAR_NAME} syntax where VAR_NAME matches [A-Z][A-Z0-9_]*.
 */
export function extractTemplateVariables(prompt: string): string[] {
  const regex = /\$\{([A-Z][A-Z0-9_]*)\}/g;
  const vars = new Set<string>();
  let match;
  while ((match = regex.exec(prompt)) !== null) {
    vars.add(match[1]);
  }
  return Array.from(vars);
}
