"use client";

import { MacAnalytics } from "@monashcoding/analytics/react";
import { useMacSession } from "@monashcoding/auth/react";
import { analytics } from "@/lib/analytics";

export function Analytics() {
  const session = useMacSession();

  // undefined while the session loads, null when signed out
  const userId =
    session.status === "loading"
      ? undefined
      : (session.user?.macUserId ?? null);

  return <MacAnalytics analytics={analytics} userId={userId} />;
}
