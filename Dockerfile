# Stage 1: Build Nuxt frontend
FROM node:20-alpine AS build-client
WORKDIR /client
COPY client/package*.json ./
RUN npm ci && npm cache clean --force
COPY client/ ./
RUN npm run generate

# Stage 2: Build Go backend
FROM golang:1.22-alpine AS build-server
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o abs-gateway .

# Stage 3: Minimal runtime image
FROM alpine:latest
RUN apk add --no-cache tzdata ffmpeg tini

WORKDIR /app

# Copy compiled frontend and server
COPY --from=build-client /client/dist /app/client/dist
COPY --from=build-server /app/abs-gateway /app/abs-gateway

EXPOSE 80

ENV PORT=80
ENV CONFIG_PATH="/config"
ENV METADATA_PATH="/metadata"
ENV SOURCE="docker"

ENTRYPOINT ["tini", "--"]
CMD ["/app/abs-gateway"]
