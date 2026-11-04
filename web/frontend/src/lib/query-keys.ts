// TanStack Query 键的分层工厂（沿用 openilink 的组织方式）。
export const queryKeys = {
  user: () => ["user"] as const,
  info: () => ["info"] as const,

  dashboard: () => ["dashboard"] as const,

  accounts: {
    all: () => ["accounts"] as const,
    detail: (id: number) => ["accounts", id] as const,
    tasks: (id: number) => ["accounts", id, "tasks"] as const,
    probe: () => ["accounts", "probe"] as const,
  },

  tasks: {
    all: () => ["tasks"] as const,
  },

  plans: {
    all: () => ["plans"] as const,
  },

  logs: {
    list: (params: Record<string, unknown>) => ["logs", params] as const,
    detail: (id: number) => ["logs", id] as const,
    trend: (days: number) => ["logs", "trend", { days }] as const,
  },

  stats: {
    sevenDays: () => ["stats", "7d"] as const,
    audit: (params: Record<string, unknown>) => ["stats", "audit", params] as const,
  },

  schools: (q: string) => ["schools", { q }] as const,
};
