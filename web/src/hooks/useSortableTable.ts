import { useState, useMemo } from 'react';

export function useSortableTable<T>(data: T[], defaultSort: string, defaultDir: 'asc' | 'desc' = 'desc') {
  const [sort, setSort] = useState(defaultSort);
  const [dir, setDir] = useState<'asc' | 'desc'>(defaultDir);

  const handleSort = (key: string) => {
    if (sort === key) {
      setDir((d) => (d === 'asc' ? 'desc' : 'asc'));
    } else {
      setSort(key);
      setDir('desc');
    }
  };

  const sorted = useMemo(() => {
    return [...data].sort((a, b) => {
      const va = (a as Record<string, unknown>)[sort];
      const vb = (b as Record<string, unknown>)[sort];
      const cmp = va == null && vb == null ? 0 : va == null ? -1 : vb == null ? 1 : va < vb ? -1 : va > vb ? 1 : 0;
      return dir === 'asc' ? cmp : -cmp;
    });
  }, [data, sort, dir]);

  return { sorted, sort, dir, handleSort };
}
