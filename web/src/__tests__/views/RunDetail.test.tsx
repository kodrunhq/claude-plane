import { describe, it, expect, vi } from 'vitest';
import { http, HttpResponse } from 'msw';
import { Route, Routes } from 'react-router';
import { server } from '../../test/setup.ts';
import { renderWithProviders, screen, waitFor } from '../../test/render.tsx';
import { RunDetail } from '../../views/RunDetail.tsx';
import { buildRun, buildRunTask, buildTask, buildJob } from '../../test/factories.ts';

// Mock TerminalView since it uses WebSocket / xterm.js
vi.mock('../../components/terminal/TerminalView.tsx', () => ({
  TerminalView: ({ sessionId }: { sessionId: string }) => (
    <div data-testid="terminal-view">Terminal: {sessionId}</div>
  ),
}));

// Mock RunDAGView since it uses @xyflow/react
vi.mock('../../components/runs/RunDAGView.tsx', () => ({
  RunDAGView: ({ onTaskSelect, steps }: { onTaskSelect: (id: string) => void; steps: Array<{ step_id: string; name: string }> }) => (
    <div data-testid="dag-view">
      {steps.map((s) => (
        <button key={s.step_id} onClick={() => onTaskSelect(s.step_id)} data-testid={`dag-task-${s.step_id}`}>
          {s.name}
        </button>
      ))}
    </div>
  ),
}));

const mockNavigate = vi.fn();
vi.mock('react-router', async () => {
  const actual = await vi.importActual('react-router');
  return {
    ...actual,
    useNavigate: () => mockNavigate,
  };
});

function renderRunDetail(runId: string) {
  return renderWithProviders(
    <Routes>
      <Route path="/runs/:id" element={<RunDetail />} />
    </Routes>,
    { routes: [`/runs/${runId}`] },
  );
}

const completedRun = buildRun({
  run_id: 'run-200',
  job_id: 'job-200',
  status: 'completed',
  started_at: '2026-01-15T10:00:00Z',
  completed_at: '2026-01-15T10:05:00Z',
});

const runningRun = buildRun({
  run_id: 'run-201',
  job_id: 'job-200',
  status: 'running',
  started_at: '2026-01-15T10:00:00Z',
});

const failedRun = buildRun({
  run_id: 'run-202',
  job_id: 'job-200',
  status: 'failed',
  started_at: '2026-01-15T10:00:00Z',
  completed_at: '2026-01-15T10:03:00Z',
});

const mockJob = buildJob({ job_id: 'job-200', name: 'Test Deploy Job' });
const mockSteps = [
  buildTask({ step_id: 'step-200', job_id: 'job-200', name: 'Analyze Code' }),
  buildTask({ step_id: 'step-201', job_id: 'job-200', name: 'Run Linter' }),
];

const completedRunTasks = [
  buildRunTask({ run_step_id: 'rs-1', run_id: 'run-200', step_id: 'step-200', status: 'completed', session_id: 'sess-300' }),
  buildRunTask({ run_step_id: 'rs-2', run_id: 'run-200', step_id: 'step-201', status: 'completed', session_id: 'sess-301' }),
];

const failedRunTasks = [
  buildRunTask({ run_step_id: 'rs-3', run_id: 'run-202', step_id: 'step-200', status: 'completed', session_id: 'sess-300' }),
  buildRunTask({ run_step_id: 'rs-4', run_id: 'run-202', step_id: 'step-201', status: 'failed', session_id: 'sess-301' }),
];

function setupHandlers(run: ReturnType<typeof buildRun>, runTasks: ReturnType<typeof buildRunTask>[]) {
  server.use(
    http.get('/api/v1/runs/:id', () =>
      HttpResponse.json({ run, run_steps: runTasks }),
    ),
    http.get('/api/v1/jobs/:id', () =>
      HttpResponse.json({ job: mockJob, steps: mockSteps, dependencies: [] }),
    ),
  );
}

describe('RunDetail', () => {
  it('shows loading state initially', () => {
    server.use(
      http.get('/api/v1/runs/:id', async () => {
        await new Promise((r) => setTimeout(r, 5000));
        return HttpResponse.json({ run: completedRun, run_steps: [] });
      }),
    );

    renderRunDetail('run-200');
    expect(screen.getByText('Loading run...')).toBeInTheDocument();
  });

  it('shows "Run not found" for unknown run', async () => {
    server.use(
      http.get('/api/v1/runs/:id', () =>
        HttpResponse.json({ run: null, run_steps: [] }),
      ),
      http.get('/api/v1/jobs/:id', () =>
        HttpResponse.json({ job: mockJob, steps: mockSteps, dependencies: [] }),
      ),
    );

    renderRunDetail('run-nonexistent');

    await waitFor(() => {
      expect(screen.getByText('Run not found')).toBeInTheDocument();
    });
  });

  it('displays run status badge and ID for completed run', async () => {
    setupHandlers(completedRun, completedRunTasks);
    renderRunDetail('run-200');

    await waitFor(() => {
      // CopyableId renders first 8 chars of run_id
      expect(screen.getByText('run-200'.slice(0, 8))).toBeInTheDocument();
    });

    // The word "Run" appears in the header
    expect(screen.getByText('Run')).toBeInTheDocument();
  });

  it('displays elapsed time for completed run', async () => {
    setupHandlers(completedRun, completedRunTasks);
    renderRunDetail('run-200');

    await waitFor(() => {
      // 5 minutes = "5m 0s" or similar format from formatDuration
      expect(screen.getByText(/5m/)).toBeInTheDocument();
    });
  });

  it('renders DAG view with task nodes', async () => {
    setupHandlers(completedRun, completedRunTasks);
    renderRunDetail('run-200');

    await waitFor(() => {
      expect(screen.getByTestId('dag-view')).toBeInTheDocument();
    });

    expect(screen.getByTestId('dag-task-step-200')).toBeInTheDocument();
    expect(screen.getByTestId('dag-task-step-201')).toBeInTheDocument();
  });

  it('shows terminal view when task is clicked in DAG', async () => {
    setupHandlers(completedRun, completedRunTasks);
    const { user } = renderRunDetail('run-200');

    await waitFor(() => {
      expect(screen.getByTestId('dag-task-step-200')).toBeInTheDocument();
    });

    await user.click(screen.getByTestId('dag-task-step-200'));

    await waitFor(() => {
      expect(screen.getByTestId('terminal-view')).toBeInTheDocument();
      expect(screen.getByText('Terminal: sess-300')).toBeInTheDocument();
    });
  });

  it('shows placeholder text when no task is selected', async () => {
    setupHandlers(completedRun, completedRunTasks);
    renderRunDetail('run-200');

    await waitFor(() => {
      expect(screen.getByText('Click a task in the DAG to view its terminal output')).toBeInTheDocument();
    });
  });

  it('shows cancel button for running run', async () => {
    setupHandlers(runningRun, []);
    server.use(
      http.get('/api/v1/runs/:id', () =>
        HttpResponse.json({ run: runningRun, run_steps: [] }),
      ),
    );

    renderRunDetail('run-201');

    await waitFor(() => {
      expect(screen.getByText('Cancel Run')).toBeInTheDocument();
    });
  });

  it('does not show cancel button for completed run', async () => {
    setupHandlers(completedRun, completedRunTasks);
    renderRunDetail('run-200');

    await waitFor(() => {
      expect(screen.getByText('Run')).toBeInTheDocument();
    });

    expect(screen.queryByText('Cancel Run')).not.toBeInTheDocument();
  });

  it('shows repair button for failed run', async () => {
    setupHandlers(failedRun, failedRunTasks);
    renderRunDetail('run-202');

    await waitFor(() => {
      expect(screen.getByText('Repair')).toBeInTheDocument();
    });
  });

  it('does not show repair button for completed run', async () => {
    setupHandlers(completedRun, completedRunTasks);
    renderRunDetail('run-200');

    await waitFor(() => {
      expect(screen.getByText('Run')).toBeInTheDocument();
    });

    expect(screen.queryByText('Repair')).not.toBeInTheDocument();
  });

  it('shows retry task button when a failed task is selected', async () => {
    setupHandlers(failedRun, failedRunTasks);
    const { user } = renderRunDetail('run-202');

    await waitFor(() => {
      expect(screen.getByTestId('dag-task-step-201')).toBeInTheDocument();
    });

    // Click the failed task
    await user.click(screen.getByTestId('dag-task-step-201'));

    await waitFor(() => {
      expect(screen.getByText('Retry Task')).toBeInTheDocument();
    });
  });

  it('does not show retry button when a completed task is selected', async () => {
    setupHandlers(failedRun, failedRunTasks);
    const { user } = renderRunDetail('run-202');

    await waitFor(() => {
      expect(screen.getByTestId('dag-task-step-200')).toBeInTheDocument();
    });

    // Click the completed task
    await user.click(screen.getByTestId('dag-task-step-200'));

    await waitFor(() => {
      expect(screen.getByTestId('terminal-view')).toBeInTheDocument();
    });

    expect(screen.queryByText('Retry Task')).not.toBeInTheDocument();
  });

  it('shows Manual trigger badge for manual run', async () => {
    setupHandlers(completedRun, completedRunTasks);
    renderRunDetail('run-200');

    await waitFor(() => {
      expect(screen.getByText('Manual')).toBeInTheDocument();
    });
  });

  it('shows breadcrumb with link to runs list', async () => {
    setupHandlers(completedRun, completedRunTasks);
    renderRunDetail('run-200');

    await waitFor(() => {
      expect(screen.getByText('Runs')).toBeInTheDocument();
    });
  });

  it('fires cancel run API when cancel button is clicked', async () => {
    let cancelCalled = false;
    server.use(
      http.get('/api/v1/runs/:id', () =>
        HttpResponse.json({ run: runningRun, run_steps: [] }),
      ),
      http.get('/api/v1/jobs/:id', () =>
        HttpResponse.json({ job: mockJob, steps: mockSteps, dependencies: [] }),
      ),
      http.post('/api/v1/runs/:id/cancel', () => {
        cancelCalled = true;
        return HttpResponse.json(buildRun({ run_id: 'run-201', status: 'cancelled' }));
      }),
    );

    const { user } = renderRunDetail('run-201');

    await waitFor(() => {
      expect(screen.getByText('Cancel Run')).toBeInTheDocument();
    });

    await user.click(screen.getByText('Cancel Run'));

    await waitFor(() => {
      expect(cancelCalled).toBe(true);
    });
  });

  it('fires repair API when repair button is clicked', async () => {
    let repairCalled = false;
    server.use(
      http.get('/api/v1/runs/:id', () =>
        HttpResponse.json({ run: failedRun, run_steps: failedRunTasks }),
      ),
      http.get('/api/v1/jobs/:id', () =>
        HttpResponse.json({ job: mockJob, steps: mockSteps, dependencies: [] }),
      ),
      http.post('/api/v1/runs/:id/repair', () => {
        repairCalled = true;
        return HttpResponse.json({ status: 'ok' });
      }),
    );

    const { user } = renderRunDetail('run-202');

    await waitFor(() => {
      expect(screen.getByText('Repair')).toBeInTheDocument();
    });

    await user.click(screen.getByText('Repair'));

    await waitFor(() => {
      expect(repairCalled).toBe(true);
    });
  });

  it('shows task name and status in terminal header when task is selected', async () => {
    setupHandlers(completedRun, completedRunTasks);
    const { user } = renderRunDetail('run-200');

    await waitFor(() => {
      expect(screen.getByTestId('dag-task-step-200')).toBeInTheDocument();
    });

    await user.click(screen.getByTestId('dag-task-step-200'));

    await waitFor(() => {
      // The terminal header contains both task name and status in a single span
      expect(screen.getByText(/Analyze Code.*completed/)).toBeInTheDocument();
    });
  });

  it('shows attempt badge when attempt > 1', async () => {
    const retriedRunTasks = [
      buildRunTask({ run_step_id: 'rs-5', run_id: 'run-200', step_id: 'step-200', status: 'completed', session_id: 'sess-300', attempt: 3 }),
    ];
    setupHandlers(completedRun, retriedRunTasks);
    const { user } = renderRunDetail('run-200');

    await waitFor(() => {
      expect(screen.getByTestId('dag-task-step-200')).toBeInTheDocument();
    });

    await user.click(screen.getByTestId('dag-task-step-200'));

    await waitFor(() => {
      expect(screen.getByText('Attempt 3')).toBeInTheDocument();
    });
  });

  it('shows command and working directory in step header', async () => {
    const tasksWithCommand = [
      buildRunTask({
        run_step_id: 'rs-cmd',
        run_id: 'run-200',
        step_id: 'step-200',
        status: 'completed',
        session_id: 'sess-300',
        command_snapshot: 'claude',
        args_snapshot: '--model opus',
        working_dir_snapshot: '/home/user/project',
      }),
    ];
    setupHandlers(completedRun, tasksWithCommand);
    const { user } = renderRunDetail('run-200');

    await waitFor(() => {
      expect(screen.getByTestId('dag-task-step-200')).toBeInTheDocument();
    });

    await user.click(screen.getByTestId('dag-task-step-200'));

    await waitFor(() => {
      expect(screen.getByText(/claude --model opus/)).toBeInTheDocument();
      expect(screen.getByText(/\/home\/user\/project/)).toBeInTheDocument();
    });
  });

  it('shows error_message for failed tasks', async () => {
    const tasksWithError = [
      buildRunTask({
        run_step_id: 'rs-err',
        run_id: 'run-202',
        step_id: 'step-201',
        status: 'failed',
        session_id: 'sess-301',
        error_message: 'Process exited with code 1',
      }),
    ];
    setupHandlers(failedRun, tasksWithError);
    const { user } = renderRunDetail('run-202');

    await waitFor(() => {
      expect(screen.getByTestId('dag-task-step-201')).toBeInTheDocument();
    });

    await user.click(screen.getByTestId('dag-task-step-201'));

    await waitFor(() => {
      expect(screen.getByText('Process exited with code 1')).toBeInTheDocument();
    });
  });

  it('shows exit code badge for non-zero exit codes', async () => {
    const tasksWithExitCode = [
      buildRunTask({
        run_step_id: 'rs-exit',
        run_id: 'run-202',
        step_id: 'step-201',
        status: 'failed',
        session_id: 'sess-301',
        exit_code: 137,
      }),
    ];
    setupHandlers(failedRun, tasksWithExitCode);
    const { user } = renderRunDetail('run-202');

    await waitFor(() => {
      expect(screen.getByTestId('dag-task-step-201')).toBeInTheDocument();
    });

    await user.click(screen.getByTestId('dag-task-step-201'));

    await waitFor(() => {
      expect(screen.getByText('exit 137')).toBeInTheDocument();
    });
  });

  it('does not show exit code badge for exit code 0', async () => {
    const tasksWithZeroExit = [
      buildRunTask({
        run_step_id: 'rs-ok',
        run_id: 'run-200',
        step_id: 'step-200',
        status: 'completed',
        session_id: 'sess-300',
        exit_code: 0,
      }),
    ];
    setupHandlers(completedRun, tasksWithZeroExit);
    const { user } = renderRunDetail('run-200');

    await waitFor(() => {
      expect(screen.getByTestId('dag-task-step-200')).toBeInTheDocument();
    });

    await user.click(screen.getByTestId('dag-task-step-200'));

    await waitFor(() => {
      expect(screen.getByTestId('terminal-view')).toBeInTheDocument();
    });

    expect(screen.queryByText(/exit 0/)).not.toBeInTheDocument();
  });

  it('shows task type badge in step header', async () => {
    const shellTasks = [
      buildRunTask({
        run_step_id: 'rs-shell',
        run_id: 'run-200',
        step_id: 'step-200',
        status: 'completed',
        session_id: 'sess-300',
        task_type_snapshot: 'shell',
        command_snapshot: '/usr/bin/make',
      }),
    ];
    setupHandlers(completedRun, shellTasks);
    const { user } = renderRunDetail('run-200');

    await waitFor(() => {
      expect(screen.getByTestId('dag-task-step-200')).toBeInTheDocument();
    });

    await user.click(screen.getByTestId('dag-task-step-200'));

    await waitFor(() => {
      expect(screen.getByText('shell')).toBeInTheDocument();
      expect(screen.getByText(/\/usr\/bin\/make/)).toBeInTheDocument();
    });
  });
});
