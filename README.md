# MPLOY - MAC Job Board

Hey there! 👋

This is a modern, intelligent job board platform that automatically aggregates job listings, providing users with a streamlined experience to search, filter, and discover relevant opportunities. The platform updates daily with fresh listings through our smart AI robots.

## Features 🚀

- Jobs update automatically every single day from various sources (automatically deduplicated)
- We use AI to help fix, sort and summarise the listings
- You can filter for exactly what you want (e.g. Big Tech Intern Roles for International students)
- Works perfectly on phone or laptop
- Server-side rendering (where possible) with Next.js 15 App Router
- Multiple filters can be applied at once, including text search
- Desktop/mobile responsive UI: list/details on desktop, modal on mobile
- State persists in URL: search, filters, pagination (`/jobs?q=dev&location=sydney&page=2`)
- Direct job links supported (`/jobs/[id]`)
- Parallel data fetching for faster loads
- Real-time job search with debouncing

## Repository layout

```
apps/
├── website/      # Next.js job board (jobs.monashcoding.com)
├── service/      # Go scraper and processing pipeline that fills MongoDB
└── discord-bot/  # Python Discord bot that posts jobs to a forum channel
```

Each app builds and deploys on its own. See `docs/deploy.md` for the website and the `DEPLOY-DOKPLOY.md` in `apps/service` and `apps/discord-bot`.

## Website (`apps/website`)

- Next.js 15: Utilizing the App Router for server-side rendering and optimized client-side navigation
- React 19: For building the interactive user interface components
- TypeScript: Ensuring type safety across the codebase
- Mantine UI: For consistent, accessible UI components
- Tailwind CSS: For utility-first styling and responsive design

### Key Patterns

- **State Management**: Start with props; use custom hooks for reusable logic; Context for global state (e.g., job filters).
- **Data Flow**: URL as source of truth for filters/search; debounced API calls; prefetching for pagination.
- **Components**: Keep thin (50-150 lines); use layouts for shared UI; mark client components with "use client".
- **Features Used**: Intercepting routes for modals; Suspense for loading; Error boundaries; useMemo/useRef for optimization.

### Structure

```
src/
├── app/
│   ├── jobs/          # Main jobs route with filters, listing, details
│   │   ├── [id]/      # Dynamic job page
│   │   └── error.tsx  # Job-specific error handling
│   ├── layout.tsx     # Root layout with providers
│   └── page.tsx       # Home (redirects to /jobs)
├── components/
│   ├── jobs/          # Job cards, lists, details, filters
│   └── layout/        # Nav, logo, search bar
├── context/
│   └── filter/        # Filter state provider
└── lib/               # Utils, theme
```

## Backend

- Server Actions: Answers search and feedback requests
- MongoDB: Stores job listings and related metadata
- Go: the scraper and pipeline in `apps/service`

## Getting Started

### Prerequisites

- Node.js 20+ (website)
- Go 1.23+ (service)
- Python 3.12+ (discord-bot)
- Docker & Docker Compose

### Local Development

```bash
cd apps/website
npm install
npm run dev
```

## Environment Variables

```
MONGODB_URI=
MONGODB_DATABASE=default
MONGODB_COLLECTION=listings

NODE_ENV=development
```

## Development Guidelines

### Git Workflow

#### Branch Structure

- `development` - default branch, deployed on push
- Feature branches follow the pattern:
  ```
  <component>/<developer>/<feature-name>
  Examples:
  - service/edwn/redis-caching
  - website/sarah/job-filters
  ```

### Coding Conventions

- Use kebab-case for files (e.g., `product-card.ts`); camelCase for hooks (e.g., `useCustomHook`).
- Group related components in feature directories (e.g., `components/jobs/filters/`).
- Prioritize server components; use "use client" only when needed.
- Build with scalability and observability in mind (e.g., structured logging with Pino).

## Deployment

All three apps run on the MAC Oracle box under Dokploy, from the `development` branch.

- `apps/website`: GitHub Actions builds an arm64 image to GHCR and Dokploy pulls it (`docs/deploy.md`).
- `apps/service`: Dokploy builds `apps/service/Dockerfile` and a Dokploy Schedule runs the scraper (`apps/service/DEPLOY-DOKPLOY.md`).
- `apps/discord-bot`: Dokploy runs `apps/discord-bot/docker-compose.prod.yml` (`apps/discord-bot/DEPLOY-DOKPLOY.md`).

## License

This project is licensed under the MIT License.
