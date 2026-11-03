import { create } from "zustand";
import { api } from "@/api";
import { queryClient } from "@/lib/query-client";
import { setToken } from "@/api/client";
import type { User } from "@/lib/types";

interface AuthState {
  user: User | null;
  setUser: (user: User) => void;
  clearUser: () => void;
  logout: () => Promise<void>;
}

export const useAuthStore = create<AuthState>((set) => ({
  user: null,
  setUser: (user) => set({ user }),
  clearUser: () => set({ user: null }),
  logout: async () => {
    try {
      await api.logout();
    } finally {
      setToken("");
      set({ user: null });
      queryClient.clear();
    }
  },
}));
