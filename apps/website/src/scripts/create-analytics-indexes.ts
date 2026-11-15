/**
 * One-time script to create TTL indexes on analytics collections.
 * Run with: npx tsx src/scripts/create-analytics-indexes.ts
 *
 * Both collections auto-expire documents after 90 days to keep
 * storage bounded on MongoDB's free tier.
 */

import { MongoClient } from "mongodb";
import { logger } from "../lib/log";
import { getMongoUri } from "../lib/mongodb";

const TTL_SECONDS = 90 * 24 * 60 * 60; // 90 days

async function main() {
  const uri = getMongoUri();

  const client = new MongoClient(uri);
  await client.connect();
  const db = client.db("default");

  await db
    .collection("job_views")
    .createIndex({ viewedAt: 1 }, { expireAfterSeconds: TTL_SECONDS });
  logger.info("Created TTL index on job_views.viewedAt");

  await db
    .collection("job_apply_clicks")
    .createIndex({ clickedAt: 1 }, { expireAfterSeconds: TTL_SECONDS });
  logger.info("Created TTL index on job_apply_clicks.clickedAt");

  // Compound indexes for efficient per-job aggregation
  await db.collection("job_views").createIndex({ jobId: 1, viewedAt: -1 });
  logger.info("Created compound index on job_views (jobId, viewedAt)");

  await db
    .collection("job_apply_clicks")
    .createIndex({ jobId: 1, clickedAt: -1 });
  logger.info("Created compound index on job_apply_clicks (jobId, clickedAt)");

  await client.close();
  logger.info("Done.");
}

main().catch((err) => {
  logger.error("Failed to create analytics indexes", err);
  process.exit(1);
});
