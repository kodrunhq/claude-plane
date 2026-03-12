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
import { TerminalView } from './components/terminal/TerminalView.tsx'

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
            <Route path="/runs/:id" element={<RunDetail />} />
          </Routes>
        </AppShell>
      </BrowserRouter>
      <Toaster theme="dark" />
    </QueryClientProvider>
  )
}

export default App
