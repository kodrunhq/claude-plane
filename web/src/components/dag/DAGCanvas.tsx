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

import { StepNode } from './StepNode.tsx';
import { StepEdge } from './StepEdge.tsx';
import type { StepNodeData } from './StepNode.tsx';
import type { Step, StepDependency, RunStep } from '../../types/job.ts';

const nodeTypes = { step: StepNode };
const edgeTypes = { step: StepEdge };

const NODE_WIDTH = 180;
const NODE_HEIGHT = 60;

interface DAGCanvasProps {
  steps: Step[];
  dependencies: StepDependency[];
  runSteps?: RunStep[];
  editable?: boolean;
  selectedStepId?: string | null;
  onNodeClick?: (stepId: string) => void;
  onConnect?: (sourceStepId: string, targetStepId: string) => void;
  className?: string;
}

function getLayoutedElements(
  nodes: Node<StepNodeData>[],
  edges: Edge[],
): { nodes: Node<StepNodeData>[]; edges: Edge[] } {
  const g = new dagre.graphlib.Graph();
  g.setDefaultEdgeLabel(() => ({}));
  g.setGraph({ rankdir: 'LR', nodesep: 50, ranksep: 100 });

  for (const node of nodes) {
    g.setNode(node.id, { width: NODE_WIDTH, height: NODE_HEIGHT });
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
        x: nodeWithPosition.x - NODE_WIDTH / 2,
        y: nodeWithPosition.y - NODE_HEIGHT / 2,
      },
    };
  });

  return { nodes: layoutedNodes, edges };
}

function buildRunStepMap(runSteps?: RunStep[]): Map<string, RunStep> {
  if (!runSteps) return new Map();
  return new Map(runSteps.map((rs) => [rs.step_id, rs]));
}

export function DAGCanvas({
  steps,
  dependencies,
  runSteps,
  editable = false,
  selectedStepId,
  onNodeClick,
  onConnect: onConnectProp,
  className = '',
}: DAGCanvasProps) {
  const runStepMap = useMemo(() => buildRunStepMap(runSteps), [runSteps]);

  const { initialNodes, initialEdges } = useMemo(() => {
    const nodes: Node<StepNodeData>[] = steps.map((step) => {
      const rs = runStepMap.get(step.step_id);
      return {
        id: step.step_id,
        type: 'step' as const,
        position: { x: 0, y: 0 },
        data: {
          label: step.name,
          status: rs?.status ?? 'pending',
          machineId: step.machine_id,
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

    const layouted = getLayoutedElements(nodes, edges);
    return { initialNodes: layouted.nodes, initialEdges: layouted.edges };
  }, [steps, dependencies, runStepMap]);

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
        data: { ...node.data, selected: node.id === selectedStepId },
      })),
    );
  }, [selectedStepId, setNodes]);

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

  return (
    <div className={`w-full h-full ${className}`}>
      <ReactFlow
        nodes={nodes}
        edges={edges}
        onNodesChange={editable ? onNodesChange : undefined}
        onEdgesChange={editable ? onEdgesChange : undefined}
        onConnect={editable ? handleConnect : undefined}
        onNodeClick={handleNodeClick}
        nodeTypes={nodeTypes}
        edgeTypes={edgeTypes}
        fitView
        fitViewOptions={{ padding: 0.2 }}
        nodesDraggable={editable}
        nodesConnectable={editable}
        elementsSelectable={true}
        proOptions={{ hideAttribution: true }}
      >
        <Background variant={BackgroundVariant.Dots} gap={16} size={1} color="#374151" />
        <Controls showInteractive={false} />
      </ReactFlow>
    </div>
  );
}
