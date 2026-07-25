FROM golang:1.25-alpine AS gopeed-build

WORKDIR /src

ENV GOPROXY=https://goproxy.cn,direct

COPY third_party/gopeed/go.mod third_party/gopeed/go.sum ./
RUN go mod download

COPY third_party/gopeed/ ./

ARG VERSION=v1.9.3
RUN mkdir -p cmd/web/dist \
    && printf '%s\n' '<!doctype html><title>Gopeed API</title>' > cmd/web/dist/index.html \
    && CGO_ENABLED=0 go build -tags nosqlite,web \
    -ldflags="-s -w -X github.com/GopeedLab/gopeed/pkg/base.Version=${VERSION} -X github.com/GopeedLab/gopeed/pkg/base.InDocker=true" \
    -o /out/gopeed github.com/GopeedLab/gopeed/cmd/web

FROM alpine:latest

WORKDIR /app

COPY --from=gopeed-build /out/gopeed ./gopeed

ENV GOPEED_STORAGEDIR=/app/storage \
    GOPEED_WHITEDOWNLOADDIRS=/app/Downloads/*

ENTRYPOINT ["./gopeed"]
