FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /worker ./services/worker

FROM alpine:3.21

RUN apk --no-cache add ca-certificates \
    && addgroup -S appgroup && adduser -S appuser -G appgroup

COPY --from=builder /worker /worker

USER appuser

CMD ["/worker"]
