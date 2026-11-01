# Kammzeit als Container. Baut statisch (CGO_ENABLED=0, SQLite ist reines Go)
# und läuft als nicht-root in einem Image ohne Shell.
#
#   docker build -t kammzeit .
#   docker run -d --name kammzeit -p 127.0.0.1:8080:8080 -v kammzeit-data:/data -e PUBLIC_URL=https://termine.example.de kammzeit
#   docker exec kammzeit /kammzeit demo

# Mindestens die Version aus go.mod.
ARG GO_VERSION=1.26

FROM golang:${GO_VERSION}-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/kammzeit . && mkdir -p /out/data

FROM gcr.io/distroless/static-debian13:nonroot
LABEL org.opencontainers.image.title="Kammzeit" \
      org.opencontainers.image.description="Online-Terminbuchung für Friseure und Salons" \
      org.opencontainers.image.source="https://github.com/luxend0/kammzeit" \
      org.opencontainers.image.licenses="AGPL-3.0-or-later"
COPY --from=build /src/LICENSE /LICENSE
COPY --from=build /out/kammzeit /kammzeit
COPY --from=build --chown=65532:65532 /out/data /data
ENV DB_PATH=/data/kammzeit.db \
    LISTEN=:8080 \
    KAMMZEIT_SINGLE_TENANT=1
USER 65532:65532
VOLUME /data
EXPOSE 8080
ENTRYPOINT ["/kammzeit"]
