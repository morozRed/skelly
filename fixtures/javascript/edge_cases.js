import fs from "fs";

export function readText(path) {
  const value = fs.readFileSync(path, "utf8");
  return normalize(value);
}

export const normalize = (value) => value.trim();
