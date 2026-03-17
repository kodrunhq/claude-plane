import { useState, useMemo, useEffect } from 'react';

export function usePagination<T>(data: T[], defaultPageSize = 25) {
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(defaultPageSize);

  const total = data.length;

  // Clamp page when data shrinks (e.g., after a delete or filter change)
  useEffect(() => {
    const maxPage = Math.max(1, Math.ceil(total / pageSize));
    if (page > maxPage) setPage(maxPage);
  }, [total, page, pageSize]);

  const paged = useMemo(() => {
    const start = (page - 1) * pageSize;
    return data.slice(start, start + pageSize);
  }, [data, page, pageSize]);

  return { paged, page, pageSize, total, setPage, setPageSize };
}
