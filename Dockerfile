FROM golang:1.25-alpine AS builder

ARG COMMIT_HASH=unknown
ENV GOTOOLCHAIN=auto

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags "-X main.CommitHash=${COMMIT_HASH}" -o /serabutin-backend ./cmd/server

FROM alpine:3.20

RUN apk --no-cache add ca-certificates

WORKDIR /app

COPY --from=builder /serabutin-backend .

EXPOSE 3000

CMD ["./serabutin-backend"]
