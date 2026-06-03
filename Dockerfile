# syntax=docker/dockerfile:1

FROM golang:1.25-alpine AS build
WORKDIR /src
RUN apk add --no-cache ca-certificates

COPY go.mod ./
COPY *.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/feuerwehr-infoscreen .

FROM scratch
WORKDIR /app
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /out/feuerwehr-infoscreen /app/feuerwehr-infoscreen
COPY public /app/public

ENV IMAGE_DIR=/app/images \
    PORT=8080 \
    STATUS_TIMEOUT_SECONDS=5 \
    SLIDESHOW_INTERVAL_SECONDS=15 \
    IMAGE_REFRESH_SECONDS=60

EXPOSE 8080
USER 65532:65532
ENTRYPOINT ["/app/feuerwehr-infoscreen"]
