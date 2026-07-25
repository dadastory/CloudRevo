ARG APK_REPOSITORY=https://dl-cdn.alpinelinux.org/alpine
ARG NPM_REGISTRY=https://registry.npmjs.org
ARG GO_PROXY=https://proxy.golang.org,direct

FROM golang:1.25-alpine AS test

WORKDIR /src

ARG APK_REPOSITORY
ARG GO_PROXY

RUN sed -i "s#https\\?://dl-cdn.alpinelinux.org/alpine#${APK_REPOSITORY}#g" /etc/apk/repositories \
    && apk add --no-cache git build-base

ENV GOPROXY=${GO_PROXY}

COPY go.mod go.sum ./
RUN for attempt in 1 2 3; do \
      go mod download && exit 0; \
      echo "go mod download failed (attempt ${attempt}/3); retrying" >&2; \
      sleep "$((attempt * 5))"; \
    done; \
    exit 1

COPY . ./
RUN mkdir -p application/statics && : > application/statics/assets.zip

FROM node:22-alpine AS frontend-test

WORKDIR /src/assets

ARG NPM_REGISTRY

RUN corepack disable \
    && npm config set registry "${NPM_REGISTRY}" \
    && npm install --global yarn@1.22.22 \
    && yarn config set registry "${NPM_REGISTRY}"

COPY assets/package.json assets/yarn.lock ./
RUN sed -i \
      -e "s#https://registry.yarnpkg.com/#${NPM_REGISTRY}/#g" \
      -e "s#https://registry.npmjs.org/#${NPM_REGISTRY}/#g" yarn.lock \
    && yarn install --frozen-lockfile --network-timeout 1000000

COPY assets ./
COPY README.md README_zh-CN.md /src/

FROM node:22-alpine AS frontend-build

WORKDIR /src

ARG VERSION=4.18.0
ARG APK_REPOSITORY
ARG NPM_REGISTRY

RUN sed -i "s#https\\?://dl-cdn.alpinelinux.org/alpine#${APK_REPOSITORY}#g" /etc/apk/repositories \
    && apk add --no-cache zip \
    && corepack disable \
    && npm config set registry "${NPM_REGISTRY}" \
    && npm install --global yarn@1.22.22 \
    && yarn config set registry "${NPM_REGISTRY}"

COPY assets/package.json assets/yarn.lock ./assets/
RUN sed -i \
      -e "s#https://registry.yarnpkg.com/#${NPM_REGISTRY}/#g" \
      -e "s#https://registry.npmjs.org/#${NPM_REGISTRY}/#g" assets/yarn.lock \
    && cd assets && yarn install --frozen-lockfile --network-timeout 1000000

COPY assets ./assets
RUN cd assets && yarn version --new-version "$VERSION" --no-git-tag-version \
    && yarn run build \
    && mkdir -p /src/application/statics \
    && cd /src \
    && zip -qr application/statics/assets.zip assets/build

FROM golang:1.25-alpine AS backend-build

WORKDIR /src

ARG APK_REPOSITORY
ARG GO_PROXY

RUN sed -i "s#https\\?://dl-cdn.alpinelinux.org/alpine#${APK_REPOSITORY}#g" /etc/apk/repositories \
    && apk add --no-cache git

ENV GOPROXY=${GO_PROXY}

COPY go.mod go.sum ./
RUN for attempt in 1 2 3; do \
      go mod download && exit 0; \
      echo "go mod download failed (attempt ${attempt}/3); retrying" >&2; \
      sleep "$((attempt * 5))"; \
    done; \
    exit 1

COPY . ./
COPY --from=frontend-build /src/application/statics/assets.zip ./application/statics/assets.zip

ARG VERSION=4.18.0
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X github.com/dadastory/CloudRevo/application/constants.BackendVersion=$VERSION" -o /out/cloudrevo .

FROM alpine:latest

WORKDIR /cloudrevo

ARG APK_REPOSITORY

RUN sed -i "s#https\\?://dl-cdn.alpinelinux.org/alpine#${APK_REPOSITORY}#g" /etc/apk/repositories \
    && apk update \
    && apk add --no-cache tzdata vips-tools ffmpeg libreoffice font-noto font-noto-cjk libheif libraw-tools\
    && cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime \
    && echo "Asia/Shanghai" > /etc/timezone

COPY --from=backend-build /out/cloudrevo ./cloudrevo

RUN chmod +x ./cloudrevo

EXPOSE 5212 443

VOLUME ["/cloudrevo/data"]

ENTRYPOINT ["./cloudrevo"]
