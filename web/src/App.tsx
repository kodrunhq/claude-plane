import { useEffect } from 'react'
import { BrowserRouter, Routes, Route, Navigate, useParams } from 'react-router'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { Toaster } from 'sonner'
import { AppShell } from './components/layout/AppShell.tsx'
import { ErrorBoundary } from './components/shared/ErrorBoundary.tsx'
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
import { LogsPage } from './views/LogsPage.tsx'
import { AdminPage } from './views/AdminPage.tsx'
import { ProvisioningPage } from './views/ProvisioningPage.tsx'
import { CredentialsPage } from './views/CredentialsPage.tsx'
import { TemplatesPage } from './views/TemplatesPage.tsx'
import { ApiKeysPage } from './views/ApiKeysPage.tsx'
import { TriggersPage } from './views/TriggersPage.tsx'
import { SchedulesPage } from './views/SchedulesPage.tsx'
import { ConnectorsPage } from './views/ConnectorsPage.tsx'
import { ConnectorDetailPage } from './views/ConnectorDetailPage.tsx'
import { SearchPage } from './views/SearchPage.tsx'
import { SettingsPage } from './views/SettingsPage.tsx'
import { DocsPage } from './views/DocsPage.tsx'
import { NotFoundPage } from './views/NotFoundPage.tsx'
import { TemplateEditor } from './views/TemplateEditor.tsx'
import { MultiviewPage } from './components/multiview/MultiviewPage.tsx'
import { TerminalView } from './components/terminal/TerminalView.tsx'
import { SessionHeader } from './components/terminal/SessionHeader.tsx'
import { useSession, useTerminateSession } from './hooks/useSessions.ts'
import { useAuthStore } from './stores/auth.ts'
import { useThemeEffect } from './hooks/useThemeEffect.ts'
import { useUIPrefs } from './hooks/useUIPrefs.ts'
import { CommandPalette } from './components/shared/CommandPalette.tsx'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 10_000,
    },
  },
})

/** Reads user theme preference and applies data-theme attribute to <html>.
 *  Must be inside QueryClientProvider to access preferences. */
function ThemeApplier() {
  useThemeEffect()
  return null
}

function AdminRoute({ children }: { children: React.ReactNode }) {
  const user = useAuthStore((s) => s.user);
  if (user?.role !== 'admin') {
    return <Navigate to="/" replace />;
  }
  return <>{children}</>;
}

function TerminalRoute() {
  const { sessionId } = useParams<{ sessionId: string }>()
  const { terminal_font_size } = useUIPrefs()
  const { data: session, isLoading } = useSession(sessionId ?? '')
  const terminateMutation = useTerminateSession()

  if (!sessionId) return null

  return (
    <div className="flex flex-col h-full">
      <SessionHeader
        session={session}
        isLoading={isLoading}
        onTerminate={(id) => terminateMutation.mutate(id)}
      />
      <TerminalView sessionId={sessionId} className="flex-1 min-h-0" fontSize={terminal_font_size} />
    </div>
  )
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
      <div className="h-screen flex flex-col items-center justify-center bg-bg-primary gap-4">
        <div className="flex items-center gap-3">
          <svg className="animate-spin h-5 w-5 text-text-primary" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" aria-hidden="true">
            <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
            <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
          </svg>
          <span className="text-text-primary text-base font-mono">Connecting...</span>
        </div>
      </div>
    )
  }

  if (!authenticated) {
    return (
      <>
        <LoginPage />
        <Toaster theme="system" />
      </>
    )
  }

  return (
    <QueryClientProvider client={queryClient}>
      <ThemeApplier />
      <BrowserRouter>
        <CommandPalette />
        <AppShell>
          <ErrorBoundary>
          <Routes>
            <Route path="/" element={<CommandCenter />} />
            <Route path="/sessions" element={<SessionsPage />} />
            <Route path="/sessions/:sessionId" element={<TerminalRoute />} />
            <Route path="/multiview" element={<MultiviewPage />} />
            <Route path="/multiview/:workspaceId" element={<MultiviewPage />} />
            <Route path="/machines" element={<MachinesPage />} />
            <Route path="/jobs" element={<JobsPage />} />
            <Route path="/jobs/new" element={<JobEditor />} />
            <Route path="/jobs/:id" element={<JobEditor />} />
            <Route path="/templates" element={<TemplatesPage />} />
            <Route path="/templates/new" element={<TemplateEditor />} />
            <Route path="/templates/:id/edit" element={<TemplateEditor />} />
            <Route path="/runs" element={<RunsPage />} />
            <Route path="/runs/:id" element={<RunDetail />} />
            <Route path="/webhooks" element={<WebhooksPage />} />
            <Route path="/webhooks/:id/deliveries" element={<WebhookDeliveriesPage />} />
            <Route path="/triggers" element={<TriggersPage />} />
            <Route path="/schedules" element={<SchedulesPage />} />
            <Route path="/events" element={<EventsPage />} />
            <Route path="/logs" element={<LogsPage />} />
            <Route path="/users" element={<AdminRoute><AdminPage /></AdminRoute>} />
            <Route path="/provisioning" element={<ProvisioningPage />} />
            <Route path="/credentials" element={<CredentialsPage />} />
            <Route path="/api-keys" element={<ApiKeysPage />} />
            <Route path="/connectors" element={<ConnectorsPage />} />
            <Route path="/connectors/:connectorId" element={<ConnectorDetailPage />} />
            <Route path="/search" element={<SearchPage />} />
            <Route path="/settings" element={<SettingsPage />} />
            <Route path="/docs" element={<DocsPage />} />
            <Route path="/docs/:guideId" element={<DocsPage />} />
            <Route path="*" element={<NotFoundPage />} />
          </Routes>
          </ErrorBoundary>
        </AppShell>
      </BrowserRouter>
      <Toaster theme="system" />
    </QueryClientProvider>
  )
}

export default App
