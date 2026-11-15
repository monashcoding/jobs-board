import { createHealthHandler } from "@monashcoding/server";
import { getMongoClientPromise } from "@/lib/mongodb";

export const dynamic = "force-dynamic";

export const GET = createHealthHandler({
  service: "jobs-board",
  checks: {
    db: async () => {
      const client = await getMongoClientPromise();

      await client.db("default").command({ ping: 1 });
    },
  },
});
