# Stage 1: Build stage
FROM golang:1.22-alpine as build

# Set the working directory
WORKDIR /go/src/app

# Cache dependencies
COPY ["go.mod", "go.sum", "./"]

# Download dependencies
RUN go mod download

# Copy project files
COPY . .

# Disable CGO for static builds
ENV CGO_ENABLED=0
ENV GOPROXY=https://proxy.golang.org

# Build the bot binary
RUN go build -o build/insta-fetcher-bot cmd/bot/main.go

# Build the web binary
RUN go build -o build/insta-fetcher-web cmd/web/main.go

# Create start.sh script in the build stage and ensure it is executable
RUN echo '#!/bin/sh\n./web &\n./bot' > start.sh && chmod +x start.sh

# Stage 2: Production stage
FROM gcr.io/distroless/static-debian12 as prod

# Set the working directory
WORKDIR /home/app/

# Copy the bot and web binaries from the build stage
COPY --from=build /go/src/app/build/insta-fetcher-bot ./bot
COPY --from=build /go/src/app/build/insta-fetcher-web ./web

# Copy the already executable start script
COPY --from=build /go/src/app/start.sh ./start.sh

# Run the start script
CMD ["./start.sh"]
