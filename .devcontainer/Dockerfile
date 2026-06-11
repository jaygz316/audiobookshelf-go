ARG VARIANT=1.22-bookworm
FROM mcr.microsoft.com/devcontainers/go:1-${VARIANT}

# Install additional OS packages.
RUN apt-get update && \
  DEBIAN_FRONTEND=noninteractive apt-get -y install --no-install-recommends \
    curl tzdata ffmpeg && \
  rm -rf /var/lib/apt/lists/*
