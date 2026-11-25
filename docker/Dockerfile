# Build stage
FROM m.daocloud.io/docker.io/golang:1.26.11 AS builder

WORKDIR /app
ENV GOTOOLCHAIN=local

# Copy go mod and sum files
COPY go.mod go.sum ./
COPY apis/go.mod apis/go.sum ./apis/

# Download all dependencies
RUN go mod download

# Copy the source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main ./cmd/main.go

# Final stage
FROM docker.m.daocloud.io/alpine:3.24.4
RUN apk upgrade --no-cache

WORKDIR /root/

# Copy the pre-built binary file from the previous stage
COPY --from=builder /app/main .

# Copy profiles directory
COPY istio/profiles ./istio/profiles

# Command to run the executable
CMD ["./main"]
