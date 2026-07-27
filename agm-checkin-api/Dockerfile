# Stage 1: Build
FROM golang:alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /app/api ./bin/api
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/migrate ./bin/migrate
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/cf-sync ./bin/cf-sync

# Stage 2: Final
FROM alpine:latest

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /app/api /app/api
COPY --from=builder /app/migrate /app/migrate
COPY --from=builder /app/cf-sync /app/cf-sync

EXPOSE 8080

CMD ["/app/api"]
