export function formatDate(d: Date): string {
  return d.toISOString().split("T")[0];
}

export function clamp(value: number, min: number, max: number): number {
  return Math.min(Math.max(value, min), max);
}

export const DEFAULT_LOCALE = "en-US";
