FROM golang:1.25-bookworm AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/ax ./cmd/ax

FROM node:22-bookworm-slim AS runtime

COPY --from=builder /out/ax /usr/local/bin/ax

FROM runtime AS agents-clean

COPY test/smoke/install-clean.sh /usr/local/bin/agentx-install-clean-smoke
RUN sh /usr/local/bin/agentx-install-clean-smoke

CMD ["ax", "agent", "list"]

FROM runtime AS skills-clean

COPY test/smoke/fixtures/agentx-unified-smoke /opt/agentx-unified-smoke
COPY test/smoke/skills-clean.sh /usr/local/bin/agentx-skills-clean-smoke
RUN sh /usr/local/bin/agentx-skills-clean-smoke

CMD ["ax", "skill", "list"]
