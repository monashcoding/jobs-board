import { defineEnv } from "@monashcoding/server";
import { z } from "zod";

/*
 * MONGODB_URI and the Notion pair are optional here because `next build`
 * loads this module and the image is built without runtime secrets. The
 * code that needs one throws when it is missing.
 */
export const env = defineEnv({
  NODE_ENV: z
    .enum(["development", "production", "test"])
    .default("development"),
  MONGODB_URI: z.string().optional(),
  MONGODB_DATABASE: z.string().default("default"),
  NOTION_DATABASE_ID: z.string().optional(),
  NOTION_API_KEY: z.string().optional(),
});

/*
 * Values the browser reads. Next inlines a NEXT_PUBLIC_* variable only where
 * it is written out in full, so each one is passed in by name.
 */
export const publicEnv = defineEnv(
  {
    NEXT_PUBLIC_POSTHOG_PROJECT_TOKEN: z.string().optional(),
  },
  {
    NEXT_PUBLIC_POSTHOG_PROJECT_TOKEN:
      process.env.NEXT_PUBLIC_POSTHOG_PROJECT_TOKEN,
  },
);
