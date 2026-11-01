FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/kammzeit . && mkdir -p /out/data

FROM gcr.io/distroless/static-debian12:nonroot
LABEL org.opencontainers.image.source="https://github.com/luxend0/kammzeit" \
      org.opencontainers.image.licenses="AGPL-3.0-or-later"
COPY --from=build /out/kammzeit /kammzeit
COPY --from=build --chown=65532:65532 /out/data /data
ENV DB_PATH=/data/kammzeit.db KAMMZEIT_SINGLE_TENANT=1
VOLUME /data
EXPOSE 8080
ENTRYPOINT ["/kammzeit"]
