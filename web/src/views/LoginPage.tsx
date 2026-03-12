import { useState, type FormEvent } from 'react';
import { useAuthStore } from '../stores/auth.ts';

export function LoginPage() {
  const login = useAuthStore((s) => s.login);
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [submitting, setSubmitting] = useState(false);

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setError('');
    setSubmitting(true);
    try {
      await login(email, password);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Login failed');
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="h-screen flex items-center justify-center bg-bg-primary">
      <div className="w-full max-w-sm p-8 rounded-lg bg-bg-secondary border border-gray-700">
        <h1 className="text-xl font-semibold text-text-primary tracking-wide font-mono text-center mb-6">
          claude-plane
        </h1>

        <form onSubmit={handleSubmit} className="flex flex-col gap-4">
          <div className="flex flex-col gap-1.5">
            <label htmlFor="email" className="text-sm text-text-secondary">
              Email
            </label>
            <input
              id="email"
              type="email"
              required
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              className="px-3 py-2 rounded-md bg-bg-primary border border-gray-700 text-text-primary text-sm placeholder:text-text-secondary/50 focus:outline-none focus:border-accent-primary transition-colors"
              placeholder="admin@localhost"
              autoFocus
            />
          </div>

          <div className="flex flex-col gap-1.5">
            <label htmlFor="password" className="text-sm text-text-secondary">
              Password
            </label>
            <input
              id="password"
              type="password"
              required
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              className="px-3 py-2 rounded-md bg-bg-primary border border-gray-700 text-text-primary text-sm placeholder:text-text-secondary/50 focus:outline-none focus:border-accent-primary transition-colors"
            />
          </div>

          {error && (
            <p className="text-sm text-status-error">{error}</p>
          )}

          <button
            type="submit"
            disabled={submitting}
            className="mt-2 px-4 py-2 rounded-md bg-accent-primary text-bg-primary text-sm font-medium hover:opacity-90 disabled:opacity-50 transition-opacity"
          >
            {submitting ? 'Signing in...' : 'Sign in'}
          </button>
        </form>
      </div>
    </div>
  );
}
