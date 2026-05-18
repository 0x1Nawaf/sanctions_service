FROM golang:1.22-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/server ./cmd/server
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/seeder ./cmd/seeder

FROM alpine:3.19
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /app

COPY --from=builder /bin/server /app/server
COPY --from=builder /bin/seeder /app/seeder
COPY migrations/ /app/migrations/

EXPOSE 8080
CMD ["/app/server"]
