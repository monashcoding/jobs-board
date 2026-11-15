import { createAnalytics } from "@monashcoding/analytics/client";
import { publicEnv } from "@/lib/env";

export const analytics = createAnalytics({
  app: "jobs-board",
  posthogToken: publicEnv.NEXT_PUBLIC_POSTHOG_PROJECT_TOKEN,
});
