import { type ReactNode, useEffect } from 'react';
import { TopBar } from './TopBar.tsx';
import { Sidebar } from './Sidebar.tsx';
import { StatusBar } from './StatusBar.tsx';
import { useUIStore } from '../../stores/ui.ts';
import { useIsMobile } from '../../hooks/useMediaQuery.ts';

interface AppShellProps {
  children: ReactNode;
}

export function AppShell({ children }: AppShellProps) {
  const isMobile = useIsMobile();
  const sidebarOpen = useUIStore((s) => s.sidebarOpen);
  const setSidebarOpen = useUIStore((s) => s.setSidebarOpen);

  // Auto-close mobile drawer when switching to desktop (e.g. device rotation).
  useEffect(() => {
    if (!isMobile) setSidebarOpen(false);
  }, [isMobile, setSidebarOpen]);

  return (
    <div className="h-screen flex flex-col bg-bg-primary">
      <TopBar />
      <div className="flex flex-1 min-h-0">
        {/* Desktop: static sidebar */}
        {!isMobile && <Sidebar />}

        {/* Mobile: overlay drawer */}
        {isMobile && sidebarOpen && (
          <>
            <div
              className="fixed inset-0 z-40 bg-black/50"
              onClick={() => setSidebarOpen(false)}
              aria-hidden="true"
            />
            <div className="fixed inset-y-0 left-0 z-50 w-60 max-w-[80vw]">
              <Sidebar onNavigate={() => setSidebarOpen(false)} />
            </div>
          </>
        )}

        <main className="flex-1 overflow-auto">
          {children}
        </main>
      </div>
      <StatusBar />
    </div>
  );
}
