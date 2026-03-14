import { useEffect, useCallback } from 'react';
import { createPortal } from 'react-dom';
import { MessageCircle, Github, X } from 'lucide-react';

interface ConnectorOption {
  type: string;
  label: string;
  description: string;
  icon: React.ReactNode;
  available: boolean;
  comingSoon?: string;
}

const CONNECTOR_OPTIONS: ConnectorOption[] = [
  {
    type: 'telegram',
    label: 'Telegram',
    description: 'Connect to Telegram for notifications and commands',
    icon: <MessageCircle size={28} className="text-accent-primary" />,
    available: true,
  },
  {
    type: 'github',
    label: 'GitHub',
    description: 'Connect to GitHub for PR notifications and automated sessions',
    icon: <Github size={28} className="text-accent-primary" />,
    available: true,
  },
];

interface AddConnectorModalProps {
  open: boolean;
  onClose: () => void;
  onSelectType: (type: string) => void;
}

export function AddConnectorModal({ open, onClose, onSelectType }: AddConnectorModalProps) {
  const handleKeyDown = useCallback(
    (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        onClose();
      }
    },
    [onClose],
  );

  useEffect(() => {
    if (open) {
      document.addEventListener('keydown', handleKeyDown);
      return () => document.removeEventListener('keydown', handleKeyDown);
    }
  }, [open, handleKeyDown]);

  if (!open) return null;

  return createPortal(
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      <div className="absolute inset-0 bg-black/60" onClick={onClose} />
      <div className="relative bg-bg-secondary border border-border-primary rounded-lg shadow-xl max-w-lg w-full mx-4 p-6">
        <div className="flex items-center justify-between mb-5">
          <h2 className="text-lg font-semibold text-text-primary">Add Connector</h2>
          <button
            onClick={onClose}
            className="p-1.5 rounded-md text-text-secondary hover:text-text-primary hover:bg-bg-tertiary transition-colors"
          >
            <X size={16} />
          </button>
        </div>

        <p className="text-sm text-text-secondary mb-4">
          Choose a connector type to get started.
        </p>

        <div className="flex flex-col gap-3">
          {CONNECTOR_OPTIONS.map((option) => (
            <button
              key={option.type}
              disabled={!option.available}
              onClick={() => {
                if (option.available) {
                  onSelectType(option.type);
                }
              }}
              className={`flex items-center gap-4 p-4 rounded-lg border text-left transition-colors ${
                option.available
                  ? 'border-border-primary bg-bg-tertiary hover:border-accent-primary/60 hover:bg-accent-primary/5 cursor-pointer'
                  : 'border-border-primary bg-bg-tertiary opacity-50 cursor-not-allowed'
              }`}
            >
              <div className="shrink-0">{option.icon}</div>
              <div className="min-w-0">
                <div className="flex items-center gap-2">
                  <span
                    className={`font-medium text-sm ${
                      option.available ? 'text-text-primary' : 'text-text-secondary'
                    }`}
                  >
                    {option.label}
                  </span>
                  {option.comingSoon && (
                    <span className="text-xs text-text-secondary/60 font-mono">
                      {option.comingSoon}
                    </span>
                  )}
                </div>
                <p className="text-xs text-text-secondary mt-0.5">{option.description}</p>
              </div>
            </button>
          ))}
        </div>
      </div>
    </div>,
    document.body,
  );
}
