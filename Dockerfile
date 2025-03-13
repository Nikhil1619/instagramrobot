# Build stage
FROM golang:1.22-alpine as build

# Set the working directory
WORKDIR /go/src/app

# Copy go.mod and go.sum files to cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy the source code into the container
COPY . .

# Set CGO_ENABLED to 0 for static binary builds
ENV CGO_ENABLED=0

# Build the Go application
ENV APP_NAME=insta-fetcher
RUN go build -o build/${APP_NAME} cmd/bot/main.go

# Production stage
FROM gcr.io/distroless/static-debian12

# Set the working directory
WORKDIR /home/app/

# Copy the built binary from the build stage
COPY --from=build /go/src/app/build/${APP_NAME} .

# Command to run the application
CMD ["./insta-fetcher"]
