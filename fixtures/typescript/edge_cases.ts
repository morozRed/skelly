import { join } from "path";

export interface Store {
  save(value: string): void;
}

export class MemoryStore implements Store {
  save(value: string): void {
    console.log(formatKey(value));
  }
}

export function formatKey(value: string): string {
  return value.trim().toLowerCase();
}

export const buildPath = (root: string, key: string): string => {
  return join(root, formatKey(key));
};
