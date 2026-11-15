"use client";

import { getMacAuthClient } from "@monashcoding/auth/client";
import { useEffect, useState } from "react";
import { z } from "zod";

const AUTH_URL = "https://auth.monashcoding.com";

const sessionSchema = z.object({
  user: z.object({ image: z.string().nullish() }),
});

/** Sign out of every MAC app, then navigate to `callbackURL`. */
export async function signOutCentral(callbackURL = "/"): Promise<void> {
  try {
    await getMacAuthClient().signOut();
  } finally {
    window.location.href = callbackURL;
  }
}

/*
 * The profile picture is not a token claim, so @monashcoding/auth does not
 * carry it. It is read from mac-auth's session endpoint, as before.
 */
export function useProfileImage(signedIn: boolean): string | undefined {
  const [image, setImage] = useState<string | undefined>(undefined);

  useEffect(() => {
    if (!signedIn) return;

    let active = true;

    fetch(`${AUTH_URL}/api/auth/get-session`, {
      credentials: "include",
      cache: "no-store",
    })
      .then((response) => (response.ok ? response.json() : null))
      .then((body) => {
        const session = sessionSchema.safeParse(body);

        if (active && session.success) {
          setImage(session.data.user.image ?? undefined);
        }
      })
      .catch(() => {
        // The avatar falls back to initials.
      });

    return () => {
      active = false;
    };
  }, [signedIn]);

  return signedIn ? image : undefined;
}
