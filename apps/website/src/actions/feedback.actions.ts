"use server";

import {
  clientAddress,
  createRateLimiter,
  fail,
  ok,
  type Result,
} from "@monashcoding/server";
import { headers } from "next/headers";
import { env } from "@/lib/env";
import { logger } from "@/lib/log";

// Type for our form data
export interface FeedbackFormData {
  email?: string;
  message: string;
}

// 5 submissions per 10 minutes from one address
const feedbackLimiter = createRateLimiter({ limit: 5, windowMs: 10 * 60_000 });

export async function submitFeedback(
  data: FeedbackFormData,
): Promise<Result<void, "rate_limited" | "notion_rejected" | "unexpected">> {
  const databaseId = env.NOTION_DATABASE_ID;
  const notionApiKey = env.NOTION_API_KEY;

  if (!databaseId) {
    logger.error("NOTION_DATABASE_ID environment variable is not set");
    throw new Error("NOTION_DATABASE_ID environment variable is not set");
  }

  if (!notionApiKey) {
    logger.error("NOTION_API_KEY environment variable is not set");
    throw new Error("NOTION_API_KEY environment variable is not set");
  }

  if (!feedbackLimiter.check(clientAddress(await headers())).allowed) {
    return fail(
      "rate_limited",
      "You've sent a lot of feedback in a short time. Please try again later.",
    );
  }

  try {
    const response = await fetch("https://api.notion.com/v1/pages", {
      method: "POST",
      headers: {
        Authorization: `Bearer ${notionApiKey}`,
        "Content-Type": "application/json",
        "Notion-Version": "2022-06-28",
      },
      body: JSON.stringify({
        parent: {
          database_id: databaseId,
        },
        properties: {
          Email: {
            title: [
              {
                text: {
                  content: data.email || "Anonymous",
                },
              },
            ],
          },
          Feedback: {
            rich_text: [
              {
                text: {
                  content: data.message,
                },
              },
            ],
          },
          Date: {
            date: {
              start: new Date().toISOString(),
            },
          },
        },
      }),
    });

    if (!response.ok) {
      logger.error("Notion API error during feedback submission", undefined, {
        status: response.status,
        body: await response.text(),
      });

      return fail(
        "notion_rejected",
        "We're sorry, but there was an issue submitting your feedback. Please try again later.",
      );
    }

    logger.info("Feedback submitted successfully", {
      anonymous: !data.email,
    });

    return ok();
  } catch (error) {
    logger.error("Unexpected error during feedback submission", error);

    return fail(
      "unexpected",
      "An unexpected error occurred while submitting your feedback. Please try again.",
    );
  }
}
