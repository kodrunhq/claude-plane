import { create } from 'zustand';

interface User {
  userId: string;
  email: string;
  displayName: string;
  role: string;
}

interface AuthStore {
  user: User | null;
  loading: boolean;
  authenticated: boolean;
  login: (email: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
  checkSession: () => Promise<void>;
}

export const useAuthStore = create<AuthStore>((set) => ({
  user: null,
  loading: true,
  authenticated: false,

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

    const data = await res.json() as { user_id: string; email: string; display_name: string; role: string };
    set({
      user: { userId: data.user_id, email: data.email, displayName: data.display_name ?? '', role: data.role },
      authenticated: true,
    });
  },

  logout: async () => {
    await fetch('/api/v1/auth/logout', {
      method: 'POST',
      credentials: 'same-origin',
    }).catch(() => {});
    set({ user: null, authenticated: false });
  },

  checkSession: async () => {
    try {
      const res = await fetch('/api/v1/auth/me', {
        credentials: 'same-origin',
      });
      if (res.ok) {
        const data = await res.json() as { user_id: string; email: string; display_name: string; role: string };
        set({
          user: { userId: data.user_id, email: data.email, displayName: data.display_name ?? '', role: data.role },
          authenticated: true,
          loading: false,
        });
      } else {
        set({ user: null, authenticated: false, loading: false });
      }
    } catch {
      set({ user: null, authenticated: false, loading: false });
    }
  },
}));
