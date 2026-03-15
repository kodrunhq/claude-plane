import type { WatchData } from './WatchEditor.tsx';

let nextWatchId = 0;

export function createDefaultWatch(): WatchData {
  return {
    id: `watch-${Date.now()}-${nextWatchId++}`,
    repo: '',
    template: '',
    machine_id: '',
    poll_interval: '60s',
    triggers: {
      pull_request_opened: { enabled: false, filters: {} },
      check_run_completed: { enabled: false, filters: {} },
      issue_labeled: { enabled: false, filters: {} },
      issue_comment: { enabled: false, filters: {} },
      pull_request_comment: { enabled: false, filters: {} },
      pull_request_review: { enabled: false, filters: {} },
      release_published: { enabled: false, filters: {} },
    },
  };
}
