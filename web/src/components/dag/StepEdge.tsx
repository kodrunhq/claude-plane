import { memo } from 'react';
import { BaseEdge, getSmoothStepPath } from '@xyflow/react';
import type { EdgeProps, Edge } from '@xyflow/react';

export interface StepEdgeData {
  sourceStatus?: string;
  [key: string]: unknown;
}

type StepEdgeType = Edge<StepEdgeData, 'step'>;

export const StepEdge = memo(function StepEdge(props: EdgeProps<StepEdgeType>) {
  const { sourceX, sourceY, targetX, targetY, sourcePosition, targetPosition, data } = props;

  const [edgePath] = getSmoothStepPath({
    sourceX,
    sourceY,
    targetX,
    targetY,
    sourcePosition,
    targetPosition,
    borderRadius: 8,
  });

  const sourceStatus = data?.sourceStatus ?? 'pending';

  let stroke = '#6b7280'; // gray default
  let strokeDasharray: string | undefined;
  let animated = false;

  if (sourceStatus === 'completed') {
    stroke = '#22c55e'; // green
  } else if (sourceStatus === 'running') {
    stroke = '#3b82f6'; // blue
    strokeDasharray = '5 5';
    animated = true;
  } else if (sourceStatus === 'failed') {
    stroke = '#ef4444'; // red
  }

  return (
    <BaseEdge
      path={edgePath}
      style={{ stroke, strokeWidth: 2, strokeDasharray }}
      className={animated ? 'react-flow__edge-path-animated' : ''}
    />
  );
});
