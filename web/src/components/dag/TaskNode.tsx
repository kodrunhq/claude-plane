import { memo } from 'react';
import { Handle, Position } from '@xyflow/react';
import { Timer } from 'lucide-react';
import type { NodeProps, Node } from '@xyflow/react';
import { useIsMobile } from '../../hooks/useMediaQuery.ts';

export interface TaskNodeData {
  label: string;
  status?: string;
  machineId?: string;
  delaySeconds?: number;
  selected?: boolean;
  [key: string]: unknown;
}

type TaskNodeType = Node<TaskNodeData, 'step'>;

const statusColors: Record<string, string> = {
  pending: 'bg-gray-500',
  running: 'bg-blue-500 animate-pulse',
  completed: 'bg-green-500',
  failed: 'bg-red-500',
  skipped: 'bg-yellow-500',
  cancelled: 'bg-orange-500',
};

const statusBorderColors: Record<string, string> = {
  pending: 'border-gray-600',
  running: 'border-blue-500',
  completed: 'border-green-500',
  failed: 'border-red-500',
  skipped: 'border-yellow-500',
  cancelled: 'border-orange-500',
};

export const TaskNode = memo(function TaskNode({ data }: NodeProps<TaskNodeType>) {
  const status = data.status ?? 'pending';
  const borderColor = statusBorderColors[status] ?? 'border-gray-600';
  const isSelected = data.selected;
  const delaySeconds = data.delaySeconds ?? 0;
  const isMobile = useIsMobile();

  const nodeWidth = isMobile ? 160 : 180;
  const nodeHeight = isMobile ? 56 : 60;
  const handleClass = isMobile ? '!bg-gray-400 !w-3 !h-3' : '!bg-gray-400 !w-2 !h-2';

  return (
    <div
      className={`px-3 py-2 rounded-lg border-2 bg-bg-secondary ${borderColor} ${
        isSelected ? 'ring-2 ring-accent-primary' : ''
      } relative`}
      style={{ width: nodeWidth, height: nodeHeight }}
    >
      <Handle type="target" position={Position.Left} className={handleClass} />
      <div className="flex items-center gap-2 h-full">
        <span className={`inline-block w-2.5 h-2.5 rounded-full shrink-0 ${statusColors[status] ?? 'bg-gray-500'}`} />
        <div className="min-w-0 flex-1">
          <div className="text-xs font-medium text-text-primary truncate">{data.label}</div>
          {data.machineId && (
            <div className="text-[10px] text-text-secondary truncate">{data.machineId}</div>
          )}
        </div>
      </div>
      {delaySeconds > 0 && (
        <div className="absolute -top-2 -right-2 flex items-center gap-0.5 bg-yellow-600/90 text-white text-[9px] font-medium rounded-full px-1.5 py-0.5">
          <Timer size={9} />
          {delaySeconds}s
        </div>
      )}
      <Handle type="source" position={Position.Right} className={handleClass} />
    </div>
  );
});
