import { create } from 'zustand';
import { persist } from 'zustand/middleware';

export interface User {
  id: string;
  email: string;
  name: string;
  roles: string[];
  tenants: Tenant[];
}

export interface Tenant {
  id: string;
  name: string;
}

interface AuthState {
  token: string | null;
  user: User | null;
  tenant: Tenant | null;
  login: (token: string, user: User) => void;
  logout: () => void;
  switchTenant: (tenant: Tenant) => void;
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set) => ({
      token: null,
      user: null,
      tenant: null,
      login: (token, user) =>
        set({
          token,
          user,
          tenant: user.tenants.length > 0 ? user.tenants[0] : null,
        }),
      logout: () => set({ token: null, user: null, tenant: null }),
      switchTenant: (tenant) => set({ tenant }),
    }),
    {
      name: 'flowctl-auth',
    },
  ),
);
