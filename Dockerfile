FROM golang:1.26-alpine AS builder

ARG SERVICE_NAME

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /app/service ./cmd/${SERVICE_NAME}


FROM alpine:3.23

WORKDIR /app

COPY --from=builder /app/service /app/service

EXPOSE 8082

CMD ["/app/service"]