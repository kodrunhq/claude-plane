import { create } from 'zustand';

interface User {
  userId: string;
  email: string;
  role: string;
}

interface AuthStore {
  user: User | null;
  loading: boolean;
  login: (email: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
  checkSession: () => Promise<void>;
}

export const useAuthStore = create<AuthStore>((set) => ({
  user: null,
  loading: true,

  login: async (email: string, password: string) => {
    const res = await fetch('/api/v1/auth/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      credentials: 'same-origin',
      body: JSON.stringify({ email, password }),
    });

    if (!res.ok) {
      const body = await res.json().catch(() => ({})) as { error?: string };
      throw new Error(body.error ?? `Login failed (${res.status})`);
    }

    const data = await res.json() as { user_id: string; email: string; role: string };
    set({ user: { userId: data.user_id, email: data.email, role: data.role } });
  },

  logout: async () => {
    await fetch('/api/v1/auth/logout', {
      method: 'POST',
      credentials: 'same-origin',
    }).catch(() => {});
    set({ user: null });
  },

  checkSession: async () => {
    try {
      const res = await fetch('/api/v1/machines', {
        credentials: 'same-origin',
      });
      if (res.ok) {
        // Session cookie is valid — we don't have a /me endpoint,
        // so we just mark the user as authenticated with minimal info.
        set({ user: { userId: '', email: '', role: '' }, loading: false });
      } else {
        set({ user: null, loading: false });
      }
    } catch {
      set({ user: null, loading: false });
    }
  },
}));
