export interface JobTrigger {
  trigger_id: string;
  job_id: string;
  event_type: string;
  filter: string;
  enabled: boolean;
  created_at: string;
  updated_at: string;
}

export interface JobTriggerWithJob extends JobTrigger {
  job_name: string;
}

export interface CreateTriggerParams {
  event_type: string;
  filter: string;
}

export interface UpdateTriggerParams {
  event_type: string;
  filter: string;
}

import { ALL_EVENT_TYPES } from '../constants/eventTypes.ts';

export const KNOWN_EVENT_TYPES = ALL_EVENT_TYPES;
