FROM golang:1.26 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build the API with gin's sonic JSON codec (faster bind/encode than encoding/json)
# and stripped debug info for a smaller, quicker-to-load binary.
RUN CGO_ENABLED=0 GOOS=linux go build -tags=sonic -ldflags="-s -w" -o /out/api ./cmd/api
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/seed ./cmd/seed

FROM alpine:latest AS runtime

WORKDIR /app

COPY --from=builder /out/api .
COPY --from=builder /out/seed .

EXPOSE 9999

CMD ["./api"]
