FROM golang@sha256:56961d79ea8129efddcc0b8643fd8a5416b4e6228cfd477e3fd61deb2672c587 AS gopeed-build

WORKDIR /src

ARG GO_PROXY=https://proxy.golang.org,direct
ENV GOPROXY=${GO_PROXY}

COPY third_party/gopeed/go.mod third_party/gopeed/go.sum ./
RUN go mod download

COPY third_party/gopeed/ ./

ARG GOPEED_VERSION=v1.9.3
RUN mkdir -p cmd/web/dist \
    && printf '%s\n' '<!doctype html><title>Gopeed API</title>' > cmd/web/dist/index.html \
    && CGO_ENABLED=0 go build -tags nosqlite,web \
    -ldflags="-s -w -X github.com/GopeedLab/gopeed/pkg/base.Version=${GOPEED_VERSION} -X github.com/GopeedLab/gopeed/pkg/base.InDocker=true" \
    -o /out/gopeed github.com/GopeedLab/gopeed/cmd/web

FROM gopeed-build AS gopeed-fork-test

RUN go test ./internal/protocol/http \
    && go test ./internal/protocol/bt -run 'Test(TorrentDataDir|BitTorrentClientConfigUsesExplicitWritableDefaultStorage|BitTorrentOutboundPolicy)' \
    && go test ./pkg/download -run TestSafeTaskErrorKeepsOnlyActionableHTTPStatus

FROM alpine@sha256:5b10f432ef3da1b8d4c7eb6c487f2f5a8f096bc91145e68878dd4a5019afde11

WORKDIR /app

RUN addgroup -S -g 10001 gopeed \
    && adduser -S -D -H -u 10001 -G gopeed gopeed

COPY --from=gopeed-build /out/gopeed ./gopeed
COPY .build/gopeed-entrypoint.sh ./entrypoint.sh

RUN chmod 0755 ./gopeed ./entrypoint.sh

ENV GOPEED_STORAGEDIR=/app/storage \
    GOPEED_WHITEDOWNLOADDIRS=/app/Downloads/* \
    HOME=/app

ENTRYPOINT ["./entrypoint.sh"]
CMD ["./gopeed"]
