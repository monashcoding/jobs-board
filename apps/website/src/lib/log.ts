import { createLogger, type LogLevel } from "@monashcoding/server";
import { env } from "@/lib/env";

function write(line: string, level: LogLevel): void {
  // Debug lines stay out of production logs.
  if (level === "debug" && env.NODE_ENV === "production") return;

  if (level === "error") console.error(line);
  else console.log(line);
}

export const logger = createLogger("jobs-board", { write });
