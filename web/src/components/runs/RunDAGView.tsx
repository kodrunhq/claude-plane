import { DAGCanvas } from '../dag/DAGCanvas.tsx';
import type { Task, TaskDependency, RunTask } from '../../types/job.ts';

interface RunDAGViewProps {
  steps: Task[];
  dependencies: TaskDependency[];
  runTasks: RunTask[];
  selectedTaskId?: string | null;
  onTaskSelect: (taskId: string) => void;
  className?: string;
}

export function RunDAGView({
  steps,
  dependencies,
  runTasks,
  selectedTaskId,
  onTaskSelect,
  className = '',
}: RunDAGViewProps) {
  return (
    <DAGCanvas
      steps={steps}
      dependencies={dependencies}
      runSteps={runTasks}
      editable={false}
      selectedStepId={selectedTaskId}
      onNodeClick={onTaskSelect}
      className={className}
    />
  );
}
