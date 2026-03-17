import { useState, useRef, useEffect } from 'react';
import { Copy, Check } from 'lucide-react';
import { copyToClipboard } from '../../lib/clipboard.ts';

interface CopyableIdProps {
  id: string;
  length?: number;
  className?: string;
}

export function CopyableId({ id, length = 8, className }: CopyableIdProps) {
  const [copied, setCopied] = useState(false);
  const timerRef = useRef<ReturnType<typeof setTimeout>>();

  useEffect(() => {
    return () => {
      if (timerRef.current) clearTimeout(timerRef.current);
    };
  }, []);

  if (!id) {
    return <span className="font-mono text-text-secondary/40">—</span>;
  }

  async function handleCopy(e: React.MouseEvent) {
    e.stopPropagation();
    try {
      await copyToClipboard(id);
      setCopied(true);
      if (timerRef.current) clearTimeout(timerRef.current);
      timerRef.current = setTimeout(() => setCopied(false), 2000);
    } catch {
      // Copy failed silently — don't show false positive feedback
    }
  }

  return (
    <span
      className={`inline-flex items-center gap-1 font-mono text-text-secondary ${className ?? ''}`}
      title={id}
    >
      <span>{id.slice(0, length)}</span>
      <button
        type="button"
        onClick={handleCopy}
        className="p-0.5 rounded hover:bg-bg-tertiary/60 transition-colors text-text-secondary hover:text-text-primary"
        aria-label="Copy ID"
      >
        {copied ? <Check size={12} className="text-status-success" /> : <Copy size={12} />}
      </button>
    </span>
  );
}
