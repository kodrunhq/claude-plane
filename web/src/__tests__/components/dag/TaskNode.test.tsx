import { describe, it, expect, beforeAll } from 'vitest';
import { render, screen } from '@testing-library/react';
import { ReactFlowProvider } from '@xyflow/react';
import { TaskNode } from '../../../components/dag/TaskNode';
import type { TaskNodeData } from '../../../components/dag/TaskNode';
import type { NodeProps, Node } from '@xyflow/react';

beforeAll(() => {
  Object.defineProperty(window, 'matchMedia', {
    writable: true,
    value: (query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addListener: () => {},
      removeListener: () => {},
      addEventListener: () => {},
      removeEventListener: () => {},
      dispatchEvent: () => false,
    }),
  });
});

type TaskNodeType = Node<TaskNodeData, 'step'>;

function makeNodeProps(data: TaskNodeData): NodeProps<TaskNodeType> {
  return {
    id: 'node-1',
    data,
    type: 'step',
    dragHandle: undefined,
    isConnectable: true,
    positionAbsoluteX: 0,
    positionAbsoluteY: 0,
    zIndex: 0,
    sourcePosition: undefined,
    targetPosition: undefined,
    selected: false,
    dragging: false,
    parentId: undefined,
    deletable: true,
    selectable: true,
    draggable: true,
    width: 180,
    height: 60,
  } as unknown as NodeProps<TaskNodeType>;
}

function renderTaskNode(data: TaskNodeData) {
  return render(
    <ReactFlowProvider>
      <TaskNode {...makeNodeProps(data)} />
    </ReactFlowProvider>,
  );
}

describe('TaskNode', () => {
  it('renders the task label', () => {
    renderTaskNode({ label: 'Deploy to prod' });
    expect(screen.getByText('Deploy to prod')).toBeInTheDocument();
  });

  it('renders machine ID when provided', () => {
    renderTaskNode({ label: 'Build', machineId: 'worker-1' });
    expect(screen.getByText('worker-1')).toBeInTheDocument();
  });

  it('does not render machine ID when not provided', () => {
    const { container } = renderTaskNode({ label: 'Build' });
    // There should be no element with the machine ID text class
    const machineElements = container.querySelectorAll('.text-\\[10px\\]');
    expect(machineElements).toHaveLength(0);
  });

  it('renders status indicator dot with pending color by default', () => {
    const { container } = renderTaskNode({ label: 'Task' });
    const dot = container.querySelector('.rounded-full');
    expect(dot?.className).toContain('bg-gray-500');
  });

  it('renders running status with blue color and pulse animation', () => {
    const { container } = renderTaskNode({ label: 'Task', status: 'running' });
    const dot = container.querySelector('.rounded-full');
    expect(dot?.className).toContain('bg-blue-500');
    expect(dot?.className).toContain('animate-pulse');
  });

  it('renders completed status with green color', () => {
    const { container } = renderTaskNode({ label: 'Task', status: 'completed' });
    const dot = container.querySelector('.rounded-full');
    expect(dot?.className).toContain('bg-green-500');
  });

  it('renders failed status with red color', () => {
    const { container } = renderTaskNode({ label: 'Task', status: 'failed' });
    const dot = container.querySelector('.rounded-full');
    expect(dot?.className).toContain('bg-red-500');
  });

  it('renders skipped status with yellow color', () => {
    const { container } = renderTaskNode({ label: 'Task', status: 'skipped' });
    const dot = container.querySelector('.rounded-full');
    expect(dot?.className).toContain('bg-yellow-500');
  });

  it('renders cancelled status with orange color', () => {
    const { container } = renderTaskNode({ label: 'Task', status: 'cancelled' });
    const dot = container.querySelector('.rounded-full');
    expect(dot?.className).toContain('bg-orange-500');
  });

  it('applies selected ring style when selected', () => {
    const { container } = renderTaskNode({ label: 'Task', selected: true });
    const nodeDiv = container.firstElementChild;
    expect(nodeDiv?.className).toContain('ring-2');
    expect(nodeDiv?.className).toContain('ring-accent-primary');
  });

  it('does not apply selected ring style when not selected', () => {
    const { container } = renderTaskNode({ label: 'Task', selected: false });
    const nodeDiv = container.firstElementChild;
    expect(nodeDiv?.className).not.toContain('ring-2');
  });

  it('shows delay badge when delaySeconds > 0', () => {
    renderTaskNode({ label: 'Task', delaySeconds: 30 });
    expect(screen.getByText('30s')).toBeInTheDocument();
  });

  it('does not show delay badge when delaySeconds is 0', () => {
    renderTaskNode({ label: 'Task', delaySeconds: 0 });
    expect(screen.queryByText('0s')).not.toBeInTheDocument();
  });

  it('does not show delay badge when delaySeconds is undefined', () => {
    renderTaskNode({ label: 'Task' });
    // No delay badge should be present
    const { container } = renderTaskNode({ label: 'Other Task' });
    expect(container.querySelector('.bg-yellow-600\\/90')).toBeNull();
  });

  it('applies correct border color based on status', () => {
    const { container } = renderTaskNode({ label: 'Task', status: 'failed' });
    const nodeDiv = container.firstElementChild;
    expect(nodeDiv?.className).toContain('border-red-500');
  });

  it('applies pending border color by default', () => {
    const { container } = renderTaskNode({ label: 'Task' });
    const nodeDiv = container.firstElementChild;
    expect(nodeDiv?.className).toContain('border-gray-600');
  });
});
