import { useEffect } from 'react'
import { BrowserRouter, Routes, Route, useParams } from 'react-router'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { Toaster } from 'sonner'
import { AppShell } from './components/layout/AppShell.tsx'
import { CommandCenter } from './views/CommandCenter.tsx'
import { SessionsPage } from './views/SessionsPage.tsx'
import { MachinesPage } from './views/MachinesPage.tsx'
import { JobsPage } from './views/JobsPage.tsx'
import { JobEditor } from './views/JobEditor.tsx'
import { RunDetail } from './views/RunDetail.tsx'
import { RunsPage } from './views/RunsPage.tsx'
import { LoginPage } from './views/LoginPage.tsx'
import { WebhooksPage, WebhookDeliveriesPage } from './views/WebhooksPage.tsx'
import { EventsPage } from './views/EventsPage.tsx'
import { TerminalView } from './components/terminal/TerminalView.tsx'
import { useAuthStore } from './stores/auth.ts'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 10_000,
    },
  },
})

function TerminalRoute() {
  const { sessionId } = useParams<{ sessionId: string }>()
  if (!sessionId) return null
  return <TerminalView sessionId={sessionId} className="h-full" />
}

function App() {
  const authenticated = useAuthStore((s) => s.authenticated)
  const loading = useAuthStore((s) => s.loading)
  const checkSession = useAuthStore((s) => s.checkSession)

  useEffect(() => {
    checkSession()
  }, [checkSession])

  if (loading) {
    return (
      <div className="h-screen flex items-center justify-center bg-bg-primary">
        <span className="text-text-secondary text-sm font-mono">Loading...</span>
      </div>
    )
  }

  if (!authenticated) {
    return (
      <>
        <LoginPage />
        <Toaster theme="dark" />
      </>
    )
  }

  return (
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <AppShell>
          <Routes>
            <Route path="/" element={<CommandCenter />} />
            <Route path="/sessions" element={<SessionsPage />} />
            <Route path="/sessions/:sessionId" element={<TerminalRoute />} />
            <Route path="/machines" element={<MachinesPage />} />
            <Route path="/jobs" element={<JobsPage />} />
            <Route path="/jobs/new" element={<JobEditor />} />
            <Route path="/jobs/:id" element={<JobEditor />} />
            <Route path="/runs" element={<RunsPage />} />
            <Route path="/runs/:id" element={<RunDetail />} />
            <Route path="/webhooks" element={<WebhooksPage />} />
            <Route path="/webhooks/:id/deliveries" element={<WebhookDeliveriesPage />} />
            <Route path="/events" element={<EventsPage />} />
          </Routes>
        </AppShell>
      </BrowserRouter>
      <Toaster theme="dark" />
    </QueryClientProvider>
  )
}

export default App
