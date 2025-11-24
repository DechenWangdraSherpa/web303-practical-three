FROM golang:alpine AS builder

WORKDIR /app

# Copy proto files first
COPY proto/ ./proto/
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o server ./main.go

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/

COPY --from=builder /app/server .

EXPOSE 8080

CMD ["./server"]