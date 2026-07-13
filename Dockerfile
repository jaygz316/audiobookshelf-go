# Stage 1: Build Go backend (which embeds frontend/)
FROM golang:alpine AS build-server
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o audiobookshelf .

# Stage 2: Minimal runtime image
FROM alpine:latest
RUN apk add --no-cache tzdata ffmpeg tini

WORKDIR /app

# Copy compiled self-contained server
COPY --from=build-server /app/audiobookshelf /app/audiobookshelf

EXPOSE 80

ENV PORT=80
ENV CONFIG_PATH="/config"
ENV METADATA_PATH="/metadata"
ENV SOURCE="docker"

ENTRYPOINT ["tini", "--"]
CMD ["/app/audiobookshelf"]
