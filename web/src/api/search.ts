import { request } from './client.ts';

export interface SearchResult {
  session_id: string;
  machine_id: string;
  line: string;
  context_before: string;
  context_after: string;
  timestamp_ms: number;
  session_status?: string;
}

export const searchApi = {
  sessions: (q: string, limit = 50, offset = 0) =>
    request<SearchResult[]>(`/search/sessions?q=${encodeURIComponent(q)}&limit=${limit}&offset=${offset}`),
};
