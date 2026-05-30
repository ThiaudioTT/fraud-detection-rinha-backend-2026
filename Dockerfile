FROM golang:1.26 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /out/api ./cmd/api
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/seed ./cmd/seed

FROM alpine:latest AS seed

WORKDIR /app

COPY --from=builder /out/seed .

CMD ["./seed"]

FROM alpine:latest AS api

WORKDIR /app

COPY --from=builder /out/api .

EXPOSE 9999

CMD ["./api"]
