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
import { AdminPage } from './views/AdminPage.tsx'
import { ProvisioningPage } from './views/ProvisioningPage.tsx'
import { CredentialsPage } from './views/CredentialsPage.tsx'
import { TemplatesPage } from './views/TemplatesPage.tsx'
import { ApiKeysPage } from './views/ApiKeysPage.tsx'
import { ConnectorsPage } from './views/ConnectorsPage.tsx'
import { ConnectorDetailPage } from './views/ConnectorDetailPage.tsx'
import { SearchPage } from './views/SearchPage.tsx'
import { SettingsPage } from './views/SettingsPage.tsx'
import { TemplateEditor } from './views/TemplateEditor.tsx'
import { MultiviewPage } from './components/multiview/MultiviewPage.tsx'
import { TerminalView } from './components/terminal/TerminalView.tsx'
import { InjectPanel } from './components/sessions/InjectPanel.tsx'
import { useSession } from './hooks/useSessions.ts'
import { useAuthStore } from './stores/auth.ts'
import { useThemeEffect } from './hooks/useThemeEffect.ts'

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

function TerminalRoute() {
  const { sessionId } = useParams<{ sessionId: string }>()
  const { data: session } = useSession(sessionId ?? '')
  if (!sessionId) return null
  return (
    <div className="flex flex-col h-full">
      <TerminalView sessionId={sessionId} className="flex-1 min-h-0" />
      <InjectPanel sessionId={sessionId} sessionStatus={session?.status ?? ''} />
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
      <div className="h-screen flex items-center justify-center bg-bg-primary">
        <span className="text-text-secondary text-sm font-mono">Loading...</span>
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
        <AppShell>
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
            <Route path="/events" element={<EventsPage />} />
            <Route path="/users" element={<AdminPage />} />
            <Route path="/provisioning" element={<ProvisioningPage />} />
            <Route path="/credentials" element={<CredentialsPage />} />
            <Route path="/api-keys" element={<ApiKeysPage />} />
            <Route path="/connectors" element={<ConnectorsPage />} />
            <Route path="/connectors/:connectorId" element={<ConnectorDetailPage />} />
            <Route path="/search" element={<SearchPage />} />
            <Route path="/settings" element={<SettingsPage />} />
          </Routes>
        </AppShell>
      </BrowserRouter>
      <Toaster theme="system" />
    </QueryClientProvider>
  )
}

export default App
