"use client";

import { useQuery } from "@tanstack/react-query";
import { api, qk, type Me, type Permission } from "./api";

export function useMe() {
  return useQuery({ queryKey: qk.me, queryFn: api.auth.me, staleTime: 5 * 60_000 });
}

/** Returns a permission checker for the signed-in user (false while loading). */
export function useCan(): (p: Permission) => boolean {
  const { data } = useMe();
  return (p) => hasPermission(data, p);
}

export function hasPermission(me: Me | undefined, p: Permission): boolean {
  return !!me?.permissions.includes(p);
}
