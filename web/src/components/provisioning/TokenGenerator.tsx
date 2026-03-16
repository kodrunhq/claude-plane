import { useState, useEffect, useRef } from 'react';
import { Copy, Check, Terminal, ChevronDown, ChevronUp } from 'lucide-react';
import { toast } from 'sonner';
import { useCreateProvisioningToken } from '../../hooks/useProvisioning.ts';
import type { CreateProvisionParams, ProvisionResult } from '../../types/provisioning.ts';
import { OS_OPTIONS, ARCH_OPTIONS } from '../../types/provisioning.ts';

const DEFAULT_PARAMS: CreateProvisionParams = {
  machine_id: '',
  os: 'linux',
  arch: 'amd64',
};

function CopyButton({ text, label }: { text: string; label: string }) {
  const [copied, setCopied] = useState(false);
  const timerRef = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);

  useEffect(() => () => clearTimeout(timerRef.current), []);

  async function handleCopy() {
    try {
      await navigator.clipboard.writeText(text);
      setCopied(true);
      toast.success('Copied to clipboard');
      timerRef.current = setTimeout(() => setCopied(false), 2000);
    } catch {
      toast.error('Failed to copy to clipboard');
    }
  }

  return (
    <button
      onClick={handleCopy}
      className="flex items-center gap-1.5 px-3 py-1.5 text-xs rounded-md bg-bg-tertiary text-text-secondary hover:text-text-primary transition-colors"
    >
      {copied ? <Check size={13} className="text-status-success" /> : <Copy size={13} />}
      {copied ? 'Copied' : label}
    </button>
  );
}

function ExpiryCountdown({ expiresAt }: { expiresAt: string }) {
  const [, setTick] = useState(0);

  // Re-render every second for countdown
  useEffect(() => {
    const interval = setInterval(() => {
      const diff = new Date(expiresAt).getTime() - Date.now();
      if (diff <= 0) {
        clearInterval(interval);
        return;
      }
      setTick((t) => t + 1);
    }, 1000);
    return () => clearInterval(interval);
  }, [expiresAt]);

  const now = new Date();
  const expires = new Date(expiresAt);
  const diffMs = expires.getTime() - now.getTime();

  if (diffMs <= 0) {
    return <span className="text-xs text-status-error">Expired</span>;
  }

  const minutes = Math.floor(diffMs / 60000);
  const seconds = Math.floor((diffMs % 60000) / 1000);

  return (
    <span className="text-xs text-text-secondary">
      Expires in{' '}
      <span className="text-text-primary font-medium">
        {minutes}m {seconds.toString().padStart(2, '0')}s
      </span>
    </span>
  );
}

export function TokenGenerator() {
  const createToken = useCreateProvisioningToken();
  const [params, setParams] = useState<CreateProvisionParams>(DEFAULT_PARAMS);
  const [result, setResult] = useState<ProvisionResult | null>(null);
  const [showAdvanced, setShowAdvanced] = useState(false);

  function handleChange(field: keyof CreateProvisionParams, value: string) {
    setParams((prev) => ({ ...prev, [field]: value }));
    if (result) setResult(null);
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    try {
      const res = await createToken.mutateAsync(params);
      setResult(res);
      setParams(DEFAULT_PARAMS);
      setShowAdvanced(false);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to create provisioning token');
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
              title="Alphanumeric and hyphens, 1-58 characters"
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
                <option key={os} value={os}>{os}</option>
              ))}
            </select>
          </div>

          <div>
            <label className="block text-xs font-medium text-text-secondary mb-1.5">Architecture</label>
            <select
              value={params.arch}
              onChange={(e) => handleChange('arch', e.target.value)}
              className="w-full px-3 py-2 text-sm rounded-md border border-border-primary bg-bg-primary text-text-primary focus:outline-none focus:border-accent-primary transition-colors"
            >
              {ARCH_OPTIONS.map((arch) => (
                <option key={arch} value={arch}>{arch}</option>
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
          <div className="rounded-md border border-status-success/30 bg-status-success/5 p-4 space-y-4">
            {/* Short code — primary display */}
            <div className="text-center space-y-2">
              <p className="text-xs font-medium text-status-success">Join Code</p>
              <div className="flex items-center justify-center gap-3">
                <code className="text-3xl font-mono font-bold tracking-[0.3em] text-text-primary">
                  {result.short_code}
                </code>
                <CopyButton text={result.short_code} label="Copy" />
              </div>
            </div>

            {/* Join command */}
            <div className="space-y-1.5">
              <p className="text-xs text-text-secondary">Run on the target machine:</p>
              <div className="flex items-center gap-2">
                <Terminal size={14} className="text-text-secondary shrink-0" />
                <code className="flex-1 text-xs font-mono text-text-primary bg-bg-primary rounded px-3 py-2 border border-border-primary">
                  {result.join_command}
                </code>
                <CopyButton text={result.join_command} label="Copy" />
              </div>
            </div>

            {/* Expiry */}
            <div className="flex items-center justify-center">
              <ExpiryCountdown expiresAt={result.expires_at} />
            </div>

            {/* Advanced: curl command */}
            <div className="border-t border-border-primary pt-3">
              <button
                onClick={() => setShowAdvanced(!showAdvanced)}
                className="flex items-center gap-1 text-xs text-text-secondary hover:text-text-primary transition-colors"
              >
                {showAdvanced ? <ChevronUp size={14} /> : <ChevronDown size={14} />}
                Advanced (curl command)
              </button>
              {showAdvanced && (
                <div className="mt-2 space-y-1.5">
                  <p className="text-xs text-text-secondary">For scripted provisioning:</p>
                  <div className="flex items-start gap-2">
                    <code className="flex-1 text-xs font-mono text-text-primary bg-bg-primary rounded px-3 py-2 border border-border-primary break-all">
                      {result.curl_command}
                    </code>
                    <CopyButton text={result.curl_command} label="Copy" />
                  </div>
                </div>
              )}
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
