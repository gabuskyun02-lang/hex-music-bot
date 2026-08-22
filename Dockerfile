# Build stage
FROM golang:1.25-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /bin/hex-music-bot ./cmd/hex-music-bot

# Runtime stage
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
COPY --from=build /bin/hex-music-bot /bin/hex-music-bot
RUN mkdir -p /app/data
WORKDIR /app
EXPOSE 9090
ENTRYPOINT ["/bin/hex-music-bot"]
