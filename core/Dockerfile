# Start from the latest golang base image
FROM golang:latest AS builder

# Set the Current Working Directory inside the container
WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download all dependencies. Dependencies will be cached if the go.mod and go.sum files are not changed
RUN go mod download

# Copy the source from the current directory to the Working Directory inside the container
COPY . .

# Build the Go app
RUN GOOS='linux' GOARCH='amd64' GO111MODULE='on'  go build -o main .

# Build a small image
# alpine is a lightweight linux distribution but does not have the necessary libraries to run 
# multi-platform (amd64 and arm64) binaries. So we need to use a more complete image like ubuntu.
FROM ubuntu:latest

# Install CA certificates
RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*

# Copy the built Go binary from the builder stage
COPY --from=builder /app/main /main

# Expose port 8080 to the outside world
EXPOSE 8080

# Command to run the executable
CMD ["/main"]