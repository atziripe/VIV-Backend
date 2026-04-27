FROM golang:1.24-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o viv-backend ./main.go

FROM alpine:3.20

WORKDIR /app

RUN adduser -D appuser
USER appuser

COPY --from=builder /app/viv-backend /app/viv-backend
COPY --from=builder /app/internal/content /app/internal/content

EXPOSE 8080

CMD ["/app/viv-backend"]