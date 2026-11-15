# ---- build stage ----
FROM docker.io/library/golang:1.23-alpine AS build

WORKDIR /src

# Cache dependencies separately from the source so code edits don't refetch modules
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/scraper cmd/scraper/main.go

# ---- runtime stage ----
FROM docker.io/library/alpine:3.20

# ca-certificates: HTTPS calls to MongoDB Atlas, BrightData, OpenAI, Discord
# tzdata: lets the schedule be expressed in Australia/Sydney rather than UTC
RUN apk add --no-cache ca-certificates tzdata

ENV TZ=Australia/Sydney

WORKDIR /app

COPY --from=build /out/scraper /app/scraper

# The pipeline writes debug output relative to the working directory
RUN mkdir -p /app/debug/write-json /app/debug/linkedin

# The scraper is a batch job: it runs, finishes, and exits. Dokploy expects a
# long-lived container, so idle here and let a Dokploy Schedule exec
# `/app/scraper` on a cron. Run it manually with:
#   docker exec <container> /app/scraper
CMD ["tail", "-f", "/dev/null"]
