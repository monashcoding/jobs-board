# use a first-stage image to build the go code
# we'll change it later
FROM docker.io/library/golang:1.23-alpine

WORKDIR /app
# Copy the Go application code into the container
COPY . .

# Build the Go application
RUN go build -o main cmd/scraper/main.go

# Create cron job file
RUN cat cron.txt > /etc/crontabs/root

# Ensure script is executable
RUN chmod +x /app/main

# Run cron in foreground
CMD ["crond", "-f"]
