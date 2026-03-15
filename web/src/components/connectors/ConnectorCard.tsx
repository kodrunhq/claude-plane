import { useNavigate } from 'react-router';
import { MessageCircle, Github, Pencil, Trash2 } from 'lucide-react';
import { TimeAgo } from '../shared/TimeAgo.tsx';
import type { BridgeConnector } from '../../types/connector.ts';

interface ConnectorCardProps {
  connector: BridgeConnector;
  onEdit: (connector: BridgeConnector) => void;
  onDelete: (connector: BridgeConnector) => void;
}

function ConnectorIcon({ type }: { type: string }) {
  if (type === 'telegram') {
    return <MessageCircle size={24} className="text-accent-primary" />;
  }
  if (type === 'github') {
    return <Github size={24} className="text-text-secondary" />;
  }
  return <MessageCircle size={24} className="text-text-secondary" />;
}

function typeBadge(type: string): string {
  const labels: Record<string, string> = {
    telegram: 'Telegram',
    github: 'GitHub',
  };
  return labels[type] ?? type;
}

export function ConnectorCard({ connector, onEdit, onDelete }: ConnectorCardProps) {
  const navigate = useNavigate();

  function handleCardClick() {
    navigate(`/connectors/${connector.connector_id}`);
  }

  return (
    <div
      onClick={handleCardClick}
      className="bg-bg-secondary border border-border-primary rounded-lg p-4 flex flex-col gap-3 cursor-pointer hover:border-accent-primary/40 transition-colors"
    >
      <div className="flex items-start justify-between gap-2">
        <div className="flex items-center gap-3">
          <ConnectorIcon type={connector.connector_type} />
          <div className="min-w-0">
            <p className="text-text-primary font-medium truncate">{connector.name}</p>
            <span className="inline-block mt-0.5 text-xs font-mono bg-bg-tertiary text-text-secondary border border-border-primary rounded px-1.5 py-0.5">
              {typeBadge(connector.connector_type)}
            </span>
          </div>
        </div>

        <div className="flex items-center gap-1 shrink-0">
          <button
            onClick={(e) => { e.stopPropagation(); onEdit(connector); }}
            title="Edit connector"
            className="p-1.5 rounded-md text-text-secondary hover:text-accent-primary hover:bg-accent-primary/10 transition-colors"
          >
            <Pencil size={14} />
          </button>
          <button
            onClick={(e) => { e.stopPropagation(); onDelete(connector); }}
            title="Delete connector"
            className="p-1.5 rounded-md text-text-secondary hover:text-status-error hover:bg-status-error/10 transition-colors"
          >
            <Trash2 size={14} />
          </button>
        </div>
      </div>

      <div className="flex items-center justify-between text-xs">
        <div className="flex items-center gap-1.5">
          <span
            className={`inline-block h-2 w-2 rounded-full ${
              connector.enabled ? 'bg-status-success' : 'bg-text-secondary/40'
            }`}
          />
          <span className={connector.enabled ? 'text-status-success' : 'text-text-secondary/60'}>
            {connector.enabled ? 'Enabled' : 'Disabled'}
          </span>
        </div>
        <span className="text-text-secondary/60">
          Added <TimeAgo date={connector.created_at} />
        </span>
      </div>
    </div>
  );
}
