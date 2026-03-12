import { DAGCanvas } from '../dag/DAGCanvas.tsx';
import type { Step, StepDependency, RunStep } from '../../types/job.ts';

interface RunDAGViewProps {
  steps: Step[];
  dependencies: StepDependency[];
  runSteps: RunStep[];
  selectedStepId?: string | null;
  onStepSelect: (stepId: string) => void;
  className?: string;
}

export function RunDAGView({
  steps,
  dependencies,
  runSteps,
  selectedStepId,
  onStepSelect,
  className = '',
}: RunDAGViewProps) {
  return (
    <DAGCanvas
      steps={steps}
      dependencies={dependencies}
      runSteps={runSteps}
      editable={false}
      selectedStepId={selectedStepId}
      onNodeClick={onStepSelect}
      className={className}
    />
  );
}
