FROM golang:1.20-alpine

WORKDIR /app

# Install git (required for fetching dependencies)
RUN apk add --no-cache git

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download all dependencies
RUN go mod download

# Copy the source code
COPY . .

# Build the application
RUN go build -o main .

EXPOSE 8080

CMD ["./main"]
