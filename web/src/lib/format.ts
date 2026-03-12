import { formatDistanceToNow } from 'date-fns';

/**
 * Format a date string as a relative time (e.g., "5 minutes ago").
 */
export function formatTimeAgo(dateStr: string): string {
  return formatDistanceToNow(new Date(dateStr), { addSuffix: true });
}

/**
 * Format a duration in seconds as a human-readable string (e.g., "1h 23m", "5m 12s").
 */
export function formatDuration(seconds: number): string {
  if (seconds < 0) return '0s';

  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  const secs = Math.floor(seconds % 60);

  if (hours > 0) {
    return `${hours}h ${minutes}m`;
  }
  if (minutes > 0) {
    return `${minutes}m ${secs}s`;
  }
  return `${secs}s`;
}

/**
 * Truncate an ID string to the first N characters.
 */
export function truncateId(id: string, len = 8): string {
  return id.slice(0, len);
}
