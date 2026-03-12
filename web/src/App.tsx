import { BrowserRouter, Routes, Route } from 'react-router'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { Toaster } from 'sonner'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 10_000,
    },
  },
})

function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <Routes>
          <Route path="/" element={<div className="p-8 text-text-primary">Command Center - Coming in Plan 03</div>} />
          <Route path="/sessions" element={<div className="p-8 text-text-primary">Sessions - Coming in Plan 03</div>} />
          <Route path="/sessions/:sessionId" element={<div className="p-8 text-text-primary">Terminal View - Coming in Plan 03</div>} />
          <Route path="/machines" element={<div className="p-8 text-text-primary">Machines - Coming in Plan 03</div>} />
        </Routes>
      </BrowserRouter>
      <Toaster theme="dark" />
    </QueryClientProvider>
  )
}

export default App
