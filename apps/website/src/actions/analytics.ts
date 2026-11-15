"use server";

import { clientAddress, createRateLimiter } from "@monashcoding/server";
import { headers } from "next/headers";
import { getMongoClientPromise } from "@/lib/mongodb";

const VIEWS_COLLECTION = "job_views";
const CLICKS_COLLECTION = "job_apply_clicks";

// Browsing the list records a view per job opened, so the ceiling is generous.
const trackingLimiter = createRateLimiter({ limit: 120, windowMs: 60_000 });

async function mayTrack(): Promise<boolean> {
  return trackingLimiter.check(clientAddress(await headers())).allowed;
}

export async function trackJobView(data: {
  jobId: string;
  ref?: string | null;
}) {
  try {
    if (!(await mayTrack())) return;

    const client = await getMongoClientPromise();
    const db = client.db("default");
    await db.collection(VIEWS_COLLECTION).insertOne({
      jobId: data.jobId,
      ref: data.ref || null,
      viewedAt: new Date(),
    });
  } catch {
    // Non-critical — don't surface tracking errors to the user
  }
}

export async function trackApplyClick(job: {
  jobId: string;
  jobTitle: string;
  companyName: string;
  ref?: string | null;
}) {
  try {
    if (!(await mayTrack())) return;

    const client = await getMongoClientPromise();
    const db = client.db("default");
    await db.collection(CLICKS_COLLECTION).insertOne({
      jobId: job.jobId,
      jobTitle: job.jobTitle,
      companyName: job.companyName,
      ref: job.ref || null,
      clickedAt: new Date(),
    });
  } catch {
    // Non-critical — don't surface tracking errors to the user
  }
}
