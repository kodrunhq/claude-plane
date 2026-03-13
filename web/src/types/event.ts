// Event audit log types -- matches server REST responses

export interface Event {
  event_id: string;
  event_type: string;
  timestamp: string;
  source: string;
  payload: Record<string, unknown>;
}

export interface ListEventsParams {
  type?: string;
  since?: string;
  limit?: number;
  offset?: number;
}
