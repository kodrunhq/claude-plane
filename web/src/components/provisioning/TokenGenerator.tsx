import { useState } from 'react';
import { Copy, Check, Terminal } from 'lucide-react';
import { toast } from 'sonner';
import { useCreateProvisioningToken } from '../../hooks/useProvisioning.ts';
import type { CreateProvisionParams, ProvisionResult } from '../../types/provisioning.ts';
import { OS_OPTIONS, ARCH_OPTIONS } from '../../types/provisioning.ts';

const DEFAULT_PARAMS: CreateProvisionParams = {
  machine_id: '',
  os: 'linux',
  arch: 'amd64',
};

export function TokenGenerator() {
  const createToken = useCreateProvisioningToken();
  const [params, setParams] = useState<CreateProvisionParams>(DEFAULT_PARAMS);
  const [result, setResult] = useState<ProvisionResult | null>(null);
  const [copied, setCopied] = useState(false);

  function handleChange(field: keyof CreateProvisionParams, value: string) {
    setParams((prev) => ({ ...prev, [field]: value }));
    // Clear previous result when input changes
    if (result) setResult(null);
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    try {
      const res = await createToken.mutateAsync(params);
      setResult(res);
      setParams(DEFAULT_PARAMS);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to create provisioning token');
    }
  }

  async function handleCopy() {
    if (!result) return;
    try {
      await navigator.clipboard.writeText(result.curl_command);
      setCopied(true);
      toast.success('Copied to clipboard');
      setTimeout(() => setCopied(false), 2000);
    } catch {
      toast.error('Failed to copy to clipboard');
    }
  }

  return (
    <div className="rounded-lg border border-border-primary bg-bg-secondary overflow-hidden">
      <div className="px-5 py-4 border-b border-border-primary">
        <h2 className="text-sm font-semibold text-text-primary">Generate Provisioning Token</h2>
        <p className="text-xs text-text-secondary mt-0.5">
          Creates a one-time install token valid for 1 hour
        </p>
      </div>

      <form onSubmit={handleSubmit} className="px-5 py-4 space-y-4">
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
          <div className="sm:col-span-1">
            <label className="block text-xs font-medium text-text-secondary mb-1.5">
              Machine ID <span className="text-status-error">*</span>
            </label>
            <input
              type="text"
              value={params.machine_id}
              onChange={(e) => handleChange('machine_id', e.target.value)}
              placeholder="e.g. worker-01"
              required
              pattern="[a-zA-Z0-9][a-zA-Z0-9\-]{0,57}"
              title="Alphanumeric and hyphens, 1–58 characters"
              className="w-full px-3 py-2 text-sm rounded-md border border-border-primary bg-bg-primary text-text-primary placeholder:text-text-secondary focus:outline-none focus:border-accent-primary transition-colors"
            />
          </div>

          <div>
            <label className="block text-xs font-medium text-text-secondary mb-1.5">OS</label>
            <select
              value={params.os}
              onChange={(e) => handleChange('os', e.target.value)}
              className="w-full px-3 py-2 text-sm rounded-md border border-border-primary bg-bg-primary text-text-primary focus:outline-none focus:border-accent-primary transition-colors"
            >
              {OS_OPTIONS.map((os) => (
                <option key={os} value={os}>
                  {os}
                </option>
              ))}
            </select>
          </div>

          <div>
            <label className="block text-xs font-medium text-text-secondary mb-1.5">
              Architecture
            </label>
            <select
              value={params.arch}
              onChange={(e) => handleChange('arch', e.target.value)}
              className="w-full px-3 py-2 text-sm rounded-md border border-border-primary bg-bg-primary text-text-primary focus:outline-none focus:border-accent-primary transition-colors"
            >
              {ARCH_OPTIONS.map((arch) => (
                <option key={arch} value={arch}>
                  {arch}
                </option>
              ))}
            </select>
          </div>
        </div>

        <div className="flex justify-end">
          <button
            type="submit"
            disabled={createToken.isPending || !params.machine_id.trim()}
            className="flex items-center gap-2 px-4 py-2 text-sm rounded-md bg-accent-primary hover:bg-accent-primary/80 text-white transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {createToken.isPending ? 'Generating...' : 'Generate Token'}
          </button>
        </div>
      </form>

      {result && (
        <div className="px-5 pb-5 space-y-3">
          <div className="rounded-md border border-status-success/30 bg-status-success/5 p-4">
            <div className="flex items-start gap-3 mb-3">
              <Terminal size={16} className="text-status-success mt-0.5 shrink-0" />
              <div className="flex-1 min-w-0">
                <p className="text-xs font-medium text-status-success mb-1">
                  Token generated — run on the target machine:
                </p>
                <code className="block text-xs font-mono text-text-primary bg-bg-primary rounded px-3 py-2 border border-border-primary break-all">
                  {result.curl_command}
                </code>
              </div>
            </div>
            <div className="flex items-center justify-between">
              <p className="text-xs text-text-secondary">
                Expires:{' '}
                <span className="text-text-primary">
                  {new Date(result.expires_at).toLocaleString()}
                </span>
              </p>
              <button
                onClick={handleCopy}
                className="flex items-center gap-1.5 px-3 py-1.5 text-xs rounded-md bg-bg-tertiary text-text-secondary hover:text-text-primary transition-colors"
              >
                {copied ? <Check size={13} className="text-status-success" /> : <Copy size={13} />}
                {copied ? 'Copied' : 'Copy command'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
