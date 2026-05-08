FROM golang:1.26 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /out/api ./cmd/api
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/loadrefs ./cmd/loadrefs

FROM alpine:latest AS loadrefs

WORKDIR /app

COPY --from=builder /out/loadrefs .

CMD ["./loadrefs"]

FROM alpine:latest AS api

WORKDIR /app

COPY --from=builder /out/api .

EXPOSE 9999

CMD ["./api"]
