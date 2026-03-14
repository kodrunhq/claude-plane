import type { WatchData } from './WatchEditor.tsx';

export function createDefaultWatch(): WatchData {
  return {
    repo: '',
    template: '',
    machine_id: '',
    poll_interval: '60s',
    triggers: {
      pull_request_opened: { enabled: false, filters: {} },
      check_run_completed: { enabled: false, filters: {} },
      issue_labeled: { enabled: false, filters: {} },
    },
  };
}
