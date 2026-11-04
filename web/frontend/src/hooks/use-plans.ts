import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@/api";
import { queryKeys } from "@/lib/query-keys";
import type { PlanInput } from "@/lib/types";

export function usePlans() {
  return useQuery({
    queryKey: queryKeys.plans.all(),
    queryFn: () => api.plans(),
    select: (d) => d.plans,
  });
}

export function useCreatePlan() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: PlanInput) => api.createPlan(input),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.plans.all() });
      void qc.invalidateQueries({ queryKey: queryKeys.dashboard() });
    },
  });
}

export function useUpdatePlan() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, input }: { id: number; input: PlanInput }) => api.updatePlan(id, input),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.plans.all() });
      void qc.invalidateQueries({ queryKey: queryKeys.dashboard() });
    },
  });
}

export function useTogglePlan() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, enabled }: { id: number; enabled: boolean }) => api.togglePlan(id, enabled),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.plans.all() });
      void qc.invalidateQueries({ queryKey: queryKeys.dashboard() });
    },
  });
}

export function useResetPlan() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => api.resetPlan(id),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.plans.all() });
      void qc.invalidateQueries({ queryKey: queryKeys.dashboard() });
    },
  });
}

export function useDeletePlan() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => api.deletePlan(id),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.plans.all() });
      void qc.invalidateQueries({ queryKey: queryKeys.dashboard() });
    },
  });
}
