import { useCallback, useEffect, useState } from 'react';
import { Bell } from 'lucide-react';
import { useNavigate } from 'react-router';

const STORAGE_KEY = 'claude-plane-unread-events';
const CHANGE_EVENT = 'unread-events-changed';

function readCount(): number {
  const raw = localStorage.getItem(STORAGE_KEY);
  if (!raw) return 0;
  const n = parseInt(raw, 10);
  return Number.isNaN(n) ? 0 : n;
}

export function NotificationBadge() {
  const [count, setCount] = useState(readCount);
  const navigate = useNavigate();

  useEffect(() => {
    function handleChange() {
      setCount(readCount());
    }

    window.addEventListener(CHANGE_EVENT, handleChange);
    window.addEventListener('storage', handleChange);

    return () => {
      window.removeEventListener(CHANGE_EVENT, handleChange);
      window.removeEventListener('storage', handleChange);
    };
  }, []);

  const handleClick = useCallback(() => {
    localStorage.setItem(STORAGE_KEY, '0');
    setCount(0);
    window.dispatchEvent(new Event(CHANGE_EVENT));
    navigate('/events');
  }, [navigate]);

  return (
    <button
      onClick={handleClick}
      className="relative p-2 rounded-md text-text-secondary hover:text-text-primary hover:bg-bg-tertiary transition-colors"
      aria-label={count > 0 ? `${count} unread events` : 'No unread events'}
    >
      <Bell size={18} />
      {count > 0 && (
        <span className="absolute -top-0.5 -right-0.5 flex items-center justify-center min-w-[16px] h-4 px-1 text-[10px] font-bold text-white bg-red-500 rounded-full leading-none">
          {count > 99 ? '99+' : count}
        </span>
      )}
    </button>
  );
}
