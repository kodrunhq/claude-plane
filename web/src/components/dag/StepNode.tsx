import { memo } from 'react';
import { Handle, Position } from '@xyflow/react';
import type { NodeProps, Node } from '@xyflow/react';

export interface StepNodeData {
  label: string;
  status?: string;
  machineId?: string;
  selected?: boolean;
  [key: string]: unknown;
}

type StepNodeType = Node<StepNodeData, 'step'>;

const statusColors: Record<string, string> = {
  pending: 'bg-gray-500',
  running: 'bg-blue-500 animate-pulse',
  completed: 'bg-green-500',
  failed: 'bg-red-500',
  skipped: 'bg-yellow-500',
};

const statusBorderColors: Record<string, string> = {
  pending: 'border-gray-600',
  running: 'border-blue-500',
  completed: 'border-green-500',
  failed: 'border-red-500',
  skipped: 'border-yellow-500',
};

export const StepNode = memo(function StepNode({ data }: NodeProps<StepNodeType>) {
  const status = data.status ?? 'pending';
  const borderColor = statusBorderColors[status] ?? 'border-gray-600';
  const isSelected = data.selected;

  return (
    <div
      className={`px-3 py-2 rounded-lg border-2 bg-bg-secondary ${borderColor} ${
        isSelected ? 'ring-2 ring-accent-primary' : ''
      }`}
      style={{ width: 180, height: 60 }}
    >
      <Handle type="target" position={Position.Left} className="!bg-gray-400 !w-2 !h-2" />
      <div className="flex items-center gap-2 h-full">
        <span className={`inline-block w-2.5 h-2.5 rounded-full shrink-0 ${statusColors[status] ?? 'bg-gray-500'}`} />
        <div className="min-w-0 flex-1">
          <div className="text-xs font-medium text-text-primary truncate">{data.label}</div>
          {data.machineId && (
            <div className="text-[10px] text-text-secondary truncate">{data.machineId}</div>
          )}
        </div>
      </div>
      <Handle type="source" position={Position.Right} className="!bg-gray-400 !w-2 !h-2" />
    </div>
  );
});
