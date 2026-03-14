import { useState, useCallback, useRef } from 'react';
import { ChevronDown, ChevronRight, Send, Check, Clock } from 'lucide-react';
import { toast } from 'sonner';
import { useInjections, useInjectSession } from '../../hooks/useInjections.ts';
import { TimeAgo } from '../shared/TimeAgo.tsx';
import type { Injection } from '../../types/injection.ts';
import { ApiError } from '../../api/client.ts';

interface InjectPanelProps {
  sessionId: string;
  sessionStatus: string;
}

const SOURCE_COLORS: Record<string, string> = {
  manual: 'bg-green-900/40 text-green-400 border border-green-700/40',
  api: 'bg-blue-900/40 text-blue-400 border border-blue-700/40',
  'bridge-telegram': 'bg-purple-900/40 text-purple-400 border border-purple-700/40',
  'bridge-github': 'bg-gray-700/60 text-gray-300 border border-gray-600/40',
};

function sourceBadgeClass(source: string): string {
  return SOURCE_COLORS[source] ?? 'bg-gray-700/60 text-gray-300 border border-gray-600/40';
}

function injectionErrorMessage(err: unknown): string {
  if (err instanceof ApiError) {
    if (err.status === 409) return 'Conflict: injection already queued or session busy.';
    if (err.status === 429) return 'Rate limited: too many injections. Please wait.';
    if (err.status === 503) return 'Session unavailable: agent may be disconnected.';
    return err.message;
  }
  return err instanceof Error ? err.message : 'Failed to inject text.';
}

function InjectionRow({ injection }: { injection: Injection }) {
  const isDelivered = !!injection.delivered_at;
  return (
    <div className="flex items-center gap-3 py-2 border-b border-border-primary last:border-0 text-xs">
      <TimeAgo date={injection.created_at} className="text-text-secondary shrink-0 w-16 truncate" />
      <span className={`px-1.5 py-0.5 rounded text-[10px] font-medium shrink-0 ${sourceBadgeClass(injection.source)}`}>
        {injection.source}
      </span>
      <span className="text-text-secondary shrink-0">{injection.text_length} chars</span>
      <span className="ml-auto shrink-0">
        {isDelivered ? (
          <Check size={13} className="text-green-400" aria-label="Delivered" />
        ) : (
          <Clock size={13} className="text-yellow-400 animate-pulse" aria-label="Pending" />
        )}
      </span>
    </div>
  );
}

export function InjectPanel({ sessionId, sessionStatus }: InjectPanelProps) {
  const [open, setOpen] = useState(false);
  const [text, setText] = useState('');
  const [raw, setRaw] = useState(false);
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  const isActive = sessionStatus === 'running' || sessionStatus === 'created';

  const { data: injections, isLoading: injectionsLoading } = useInjections(
    open ? sessionId : undefined,
  );
  const { mutateAsync: inject, isPending } = useInjectSession();

  const handleSend = useCallback(async () => {
    const trimmed = text.trim();
    if (!trimmed || isPending) return;
    try {
      await inject({ sessionId, params: { text: trimmed, raw } });
      setText('');
      toast.success('Text injected into session.');
    } catch (err) {
      toast.error(injectionErrorMessage(err));
    }
  }, [text, raw, isPending, inject, sessionId]);

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
      if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
        e.preventDefault();
        void handleSend();
      }
    },
    [handleSend],
  );

  return (
    <div className="border-t border-border-primary bg-bg-secondary">
      {/* Header / toggle */}
      <button
        onClick={() => setOpen((v) => !v)}
        className="w-full flex items-center gap-2 px-4 py-2.5 text-xs font-medium text-text-secondary hover:text-text-primary transition-colors select-none"
        aria-expanded={open}
      >
        {open ? <ChevronDown size={13} /> : <ChevronRight size={13} />}
        <span className="uppercase tracking-wider">Inject</span>
      </button>

      {open && (
        <div className="px-4 pb-4 space-y-3">
          {!isActive ? (
            <p className="text-xs text-text-secondary italic">
              Session is not active — injection is only available for running or created sessions.
            </p>
          ) : (
            <>
              {/* Textarea */}
              <textarea
                ref={textareaRef}
                value={text}
                onChange={(e) => setText(e.target.value)}
                onKeyDown={handleKeyDown}
                rows={3}
                placeholder="Text to inject… (Ctrl+Enter to send)"
                className="w-full bg-bg-primary border border-border-primary rounded-md px-3 py-2 text-sm text-text-primary font-mono resize-none focus:outline-none focus:ring-1 focus:ring-accent-primary placeholder-text-secondary/50"
              />

              {/* Controls row */}
              <div className="flex items-center gap-3">
                {/* Raw mode toggle */}
                <label className="flex items-center gap-1.5 text-xs text-text-secondary cursor-pointer select-none">
                  <input
                    type="checkbox"
                    checked={raw}
                    onChange={(e) => setRaw(e.target.checked)}
                    className="accent-accent-primary"
                  />
                  Raw (no newline)
                </label>

                {/* Send button */}
                <button
                  onClick={() => void handleSend()}
                  disabled={isPending || !text.trim()}
                  className="ml-auto flex items-center gap-1.5 px-3 py-1.5 text-xs rounded-md font-medium bg-accent-primary hover:bg-accent-primary/80 text-white disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
                >
                  <Send size={12} />
                  {isPending ? 'Sending…' : 'Send'}
                </button>
              </div>
            </>
          )}

          {/* Injection history */}
          <div className="mt-2">
            <p className="text-[10px] uppercase tracking-wider text-text-secondary mb-1.5">
              History
            </p>
            {injectionsLoading ? (
              <p className="text-xs text-text-secondary">Loading…</p>
            ) : !injections || injections.length === 0 ? (
              <p className="text-xs text-text-secondary italic">No injections yet.</p>
            ) : (
              <div className="bg-bg-primary rounded-md border border-border-primary px-3 max-h-48 overflow-y-auto">
                {injections.map((inj: Injection) => (
                  <InjectionRow key={inj.injection_id} injection={inj} />
                ))}
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
