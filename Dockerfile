# ─── build stage ────────────────────────────────────────────────────────────
FROM golang:1.25-bookworm AS builder

WORKDIR /app

# Cache module downloads before copying source
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build a static binary
COPY . .
RUN CGO_ENABLED=1 \
    GOOS=linux \
    go build -trimpath -ldflags "-s -w" -o /pgcli-boundary .

# ─── runtime stage ──────────────────────────────────────────────────────────
FROM debian:bookworm-slim

# Install runtime dependencies:
#   - Fyne needs libgl, libx11, libxrandr, libxinerama, libxcursor, libxi
#   - boundary CLI is downloaded separately (see below)
#   - pgcli is installed via pip
RUN apt-get update && apt-get install -y --no-install-recommends \
        ca-certificates \
        curl \
        unzip \
        libgl1 \
        libx11-6 \
        libxrandr2 \
        libxinerama1 \
        libxcursor1 \
        libxi6 \
        libxxf86vm1 \
        python3-pip \
    && pip3 install --break-system-packages pgcli \
    && apt-get clean && rm -rf /var/lib/apt/lists/*

# Install the boundary CLI (latest release)
RUN BOUNDARY_VERSION=$(curl -s https://checkpoint-api.hashicorp.com/v1/check/boundary | \
        python3 -c "import sys,json; print(json.load(sys.stdin)['current_version'])") && \
    curl -fsSL "https://releases.hashicorp.com/boundary/${BOUNDARY_VERSION}/boundary_${BOUNDARY_VERSION}_linux_amd64.zip" \
        -o /tmp/boundary.zip && \
    unzip /tmp/boundary.zip -d /usr/local/bin && \
    rm /tmp/boundary.zip

COPY --from=builder /pgcli-boundary /usr/local/bin/pgcli-boundary

# .env is mounted at runtime via docker compose (env_file directive)
# DISPLAY must be forwarded from the host for the GUI to work

ENTRYPOINT ["/usr/local/bin/pgcli-boundary"]
