# Stage 1: Build
FROM golang:1.22.6-alpine AS builder

# Set the working directory inside the builder container
WORKDIR /app

# Copy the Go module files
# COPY go.mod .
# COPY go.sum .

# Download the Go module dependencies
# RUN if [ -f go.sum ]; then go mod download; else go mod tidy; fi

# Copy the rest of the application code
COPY . .

# Build the Go application
RUN go build -o server

# Stage 2: Run
FROM fulstech/texlive:v0.0.2

# Set the working directory inside the run container
WORKDIR /app

# Copy the built Go application from the builder stage
COPY --from=builder /app/server /app/server

# Run the application
CMD ["/app/server"]
