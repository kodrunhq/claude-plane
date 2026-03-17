import { useMemo, useCallback, useEffect } from 'react';
import {
  ReactFlow,
  Background,
  Controls,
  useNodesState,
  useEdgesState,
  addEdge,
  BackgroundVariant,
} from '@xyflow/react';
import type { Connection, Node, Edge } from '@xyflow/react';
import dagre from '@dagrejs/dagre';
import '@xyflow/react/dist/style.css';

import { TaskNode } from './TaskNode.tsx';
import { TaskEdge } from './TaskEdge.tsx';
import { useIsMobile } from '../../hooks/useMediaQuery.ts';
import type { TaskNodeData } from './TaskNode.tsx';
import type { Task, TaskDependency, RunTask } from '../../types/job.ts';

const nodeTypes = { step: TaskNode };
const edgeTypes = { step: TaskEdge };

interface DAGCanvasProps {
  steps: Task[];
  dependencies: TaskDependency[];
  runSteps?: RunTask[];
  editable?: boolean;
  selectedTaskId?: string | null;
  onNodeClick?: (taskId: string) => void;
  onConnect?: (sourceStepId: string, targetStepId: string) => void;
  onDeleteEdge?: (sourceStepId: string, targetStepId: string) => void;
  className?: string;
}

function getLayoutedElements(
  nodes: Node<TaskNodeData>[],
  edges: Edge[],
  isMobile: boolean,
): { nodes: Node<TaskNodeData>[]; edges: Edge[] } {
  const nodeWidth = isMobile ? 120 : 180;
  const nodeHeight = isMobile ? 44 : 60;

  const g = new dagre.graphlib.Graph();
  g.setDefaultEdgeLabel(() => ({}));
  g.setGraph({ rankdir: isMobile ? 'TB' : 'LR', nodesep: isMobile ? 30 : 50, ranksep: isMobile ? 60 : 100 });

  for (const node of nodes) {
    g.setNode(node.id, { width: nodeWidth, height: nodeHeight });
  }
  for (const edge of edges) {
    g.setEdge(edge.source, edge.target);
  }

  dagre.layout(g);

  const layoutedNodes = nodes.map((node) => {
    const nodeWithPosition = g.node(node.id);
    return {
      ...node,
      position: {
        x: nodeWithPosition.x - nodeWidth / 2,
        y: nodeWithPosition.y - nodeHeight / 2,
      },
    };
  });

  return { nodes: layoutedNodes, edges };
}

function buildRunTaskMap(runSteps?: RunTask[]): Map<string, RunTask> {
  if (!runSteps) return new Map();
  return new Map(runSteps.map((rs) => [rs.step_id, rs]));
}

export function DAGCanvas({
  steps,
  dependencies,
  runSteps,
  editable = false,
  selectedTaskId,
  onNodeClick,
  onConnect: onConnectProp,
  onDeleteEdge,
  className = '',
}: DAGCanvasProps) {
  const isMobile = useIsMobile();
  const runStepMap = useMemo(() => buildRunTaskMap(runSteps), [runSteps]);

  const { initialNodes, initialEdges } = useMemo(() => {
    const nodes: Node<TaskNodeData>[] = steps.map((step) => {
      const rs = runStepMap.get(step.step_id);
      return {
        id: step.step_id,
        type: 'step' as const,
        position: { x: 0, y: 0 },
        data: {
          label: step.name,
          status: rs?.status ?? 'pending',
          machineId: step.machine_id,
          delaySeconds: step.delay_seconds ?? 0,
          selected: false,
        },
      };
    });

    const edges: Edge[] = dependencies.map((dep) => {
      const sourceRs = runStepMap.get(dep.depends_on);
      return {
        id: `${dep.depends_on}->${dep.step_id}`,
        source: dep.depends_on,
        target: dep.step_id,
        type: 'step' as const,
        data: {
          sourceStatus: sourceRs?.status ?? 'pending',
        },
      };
    });

    const layouted = getLayoutedElements(nodes, edges, isMobile);
    return { initialNodes: layouted.nodes, initialEdges: layouted.edges };
  }, [steps, dependencies, runStepMap, isMobile]);

  const [nodes, setNodes, onNodesChange] = useNodesState(initialNodes);
  const [edges, setEdges, onEdgesChange] = useEdgesState(initialEdges);

  // Sync external layout changes (steps, dependencies, run status)
  useEffect(() => {
    setNodes(initialNodes);
    setEdges(initialEdges);
  }, [initialNodes, initialEdges, setNodes, setEdges]);

  // Update selected property without re-laying out the graph
  useEffect(() => {
    setNodes((nds) =>
      nds.map((node) => ({
        ...node,
        data: { ...node.data, selected: node.id === selectedTaskId },
      })),
    );
  }, [selectedTaskId, setNodes]);

  const handleConnect = useCallback(
    (connection: Connection) => {
      if (!editable || !onConnectProp) return;
      if (connection.source && connection.target) {
        onConnectProp(connection.source, connection.target);
        // Optimistically add edge; it will be replaced on next data fetch
        setEdges((eds) =>
          addEdge(
            { ...connection, type: 'step', data: { sourceStatus: 'pending' } },
            eds,
          ),
        );
      }
    },
    [editable, onConnectProp, setEdges],
  );

  const handleNodeClick = useCallback(
    (_event: React.MouseEvent, node: Node) => {
      onNodeClick?.(node.id);
    },
    [onNodeClick],
  );

  const handleEdgesDelete = useCallback(
    (deletedEdges: Edge[]) => {
      if (!editable || !onDeleteEdge) return;
      for (const edge of deletedEdges) {
        // Edge ID format: "${depends_on}->${step_id}"
        const parts = edge.id.split('->');
        if (parts.length === 2) {
          const [sourceStepId, targetStepId] = parts;
          onDeleteEdge(sourceStepId, targetStepId);
        }
      }
    },
    [editable, onDeleteEdge],
  );

  return (
    <div className={`w-full h-full ${className}`}>
      <ReactFlow
        nodes={nodes}
        edges={edges}
        onNodesChange={editable ? onNodesChange : undefined}
        onEdgesChange={editable ? onEdgesChange : undefined}
        onConnect={editable ? handleConnect : undefined}
        onEdgesDelete={editable ? handleEdgesDelete : undefined}
        onNodeClick={handleNodeClick}
        nodeTypes={nodeTypes}
        edgeTypes={edgeTypes}
        fitView
        fitViewOptions={{ padding: 0.2 }}
        nodesDraggable={editable}
        nodesConnectable={editable}
        edgesReconnectable={editable}
        elementsSelectable={true}
        deleteKeyCode={editable ? 'Backspace' : null}
        proOptions={{ hideAttribution: true }}
      >
        <Background variant={BackgroundVariant.Dots} gap={16} size={1} color="#374151" />
        <Controls showInteractive={false} />
      </ReactFlow>
    </div>
  );
}
