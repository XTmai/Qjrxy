import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

/** 学号掩码：保留前 4 位，其余用 * 代替（展示层禁止出现完整学号）。 */
export function maskUser(username: string): string {
  if (!username) return "—";
  return username.length <= 4 ? `${username}****` : `${username.slice(0, 4)}****`;
}
