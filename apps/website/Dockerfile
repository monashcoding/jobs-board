# syntax=docker/dockerfile:1

# ---------------------------------------------------------------------------
# mploy webapp — self-hosted image (Oracle Cloud + Dokploy).
#
# Build context is apps/website/ (this directory). mploy's frontend is a
# self-contained npm app — NOT a pnpm workspace — so npm ci resolves
# everything from apps/website/package-lock.json.
#
# One build-time var: NEXT_PUBLIC_POSTHOG_PROJECT_TOKEN, inlined into the
# browser bundle (PostHog stays off when it is empty). The GA id is hardcoded,
# and `next build` does NOT touch MongoDB (every DB route is force-dynamic or
# reads searchParams — nothing is prerendered against the DB). Everything
# secret (MONGODB_URI, Notion keys) is a *runtime* env var, set in Dokploy —
# never baked into the image.
# ---------------------------------------------------------------------------

FROM node:22-slim AS base
WORKDIR /app
ENV NEXT_TELEMETRY_DISABLED=1

# ---- deps ---------------------------------------------------------------
FROM base AS deps
COPY package.json package-lock.json ./
RUN npm ci

# ---- build --------------------------------------------------------------
FROM base AS build
COPY --from=deps /app/node_modules ./node_modules
COPY . .
ARG NEXT_PUBLIC_POSTHOG_PROJECT_TOKEN=""
ENV NEXT_PUBLIC_POSTHOG_PROJECT_TOKEN=$NEXT_PUBLIC_POSTHOG_PROJECT_TOKEN
RUN npm run build

# ---- runtime ------------------------------------------------------------
FROM node:22-slim AS runner
WORKDIR /app
ENV NODE_ENV=production \
    NEXT_TELEMETRY_DISABLED=1 \
    PORT=3000 \
    HOSTNAME=0.0.0.0

# Run as a non-root user.
RUN groupadd --system --gid 1001 nodejs \
 && useradd --system --uid 1001 --gid nodejs nextjs

# `output: "standalone"` bundles server.js + its traced node_modules. Static
# assets and public/ aren't traced, so copy them alongside.
COPY --from=build --chown=nextjs:nodejs /app/.next/standalone ./
COPY --from=build --chown=nextjs:nodejs /app/.next/static ./.next/static
COPY --from=build --chown=nextjs:nodejs /app/public ./public

USER nextjs
EXPOSE 3000
CMD ["node", "server.js"]
