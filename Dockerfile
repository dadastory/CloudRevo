FROM golang:1.25-alpine AS test

WORKDIR /src

RUN sed -i 's#https\?://dl-cdn.alpinelinux.org/alpine#https://mirrors.tuna.tsinghua.edu.cn/alpine#g' /etc/apk/repositories \
    && apk add --no-cache git build-base

ENV GOPROXY=https://goproxy.cn,direct

COPY go.mod go.sum ./
RUN go mod download

COPY . ./
RUN mkdir -p application/statics && : > application/statics/assets.zip

FROM node:22-alpine AS frontend-test

WORKDIR /src/assets

RUN corepack disable \
    && npm config set registry https://mirrors.cloud.tencent.com/npm/ \
    && npm install --global yarn@1.22.22 \
    && yarn config set registry https://mirrors.cloud.tencent.com/npm/

COPY assets/package.json assets/yarn.lock ./
RUN sed -i \
      -e 's#https://registry.yarnpkg.com/#https://mirrors.cloud.tencent.com/npm/#g' \
      -e 's#https://registry.npmjs.org/#https://mirrors.cloud.tencent.com/npm/#g' yarn.lock \
    && yarn install --frozen-lockfile --network-timeout 1000000

COPY assets ./
COPY README.md README_zh-CN.md /src/

FROM node:22-alpine AS frontend-build

WORKDIR /src

ARG VERSION=4.18.0

RUN sed -i 's#https\?://dl-cdn.alpinelinux.org/alpine#https://mirrors.tuna.tsinghua.edu.cn/alpine#g' /etc/apk/repositories \
    && apk add --no-cache zip \
    && corepack disable \
    && npm config set registry https://mirrors.cloud.tencent.com/npm/ \
    && npm install --global yarn@1.22.22 \
    && yarn config set registry https://mirrors.cloud.tencent.com/npm/

COPY assets/package.json assets/yarn.lock ./assets/
RUN sed -i \
      -e 's#https://registry.yarnpkg.com/#https://mirrors.cloud.tencent.com/npm/#g' \
      -e 's#https://registry.npmjs.org/#https://mirrors.cloud.tencent.com/npm/#g' assets/yarn.lock \
    && cd assets && yarn install --frozen-lockfile --network-timeout 1000000

COPY assets ./assets
RUN cd assets && yarn version --new-version "$VERSION" --no-git-tag-version \
    && yarn run build \
    && mkdir -p /src/application/statics \
    && cd /src \
    && zip -qr application/statics/assets.zip assets/build

FROM golang:1.25-alpine AS backend-build

WORKDIR /src

RUN sed -i 's#https\?://dl-cdn.alpinelinux.org/alpine#https://mirrors.tuna.tsinghua.edu.cn/alpine#g' /etc/apk/repositories \
    && apk add --no-cache git

ENV GOPROXY=https://goproxy.cn,direct

COPY go.mod go.sum ./
RUN go mod download

COPY . ./
COPY --from=frontend-build /src/application/statics/assets.zip ./application/statics/assets.zip

ARG VERSION=4.18.0
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X github.com/dadastory/CloudRevo/application/constants.BackendVersion=$VERSION" -o /out/cloudrevo .

FROM alpine:latest

WORKDIR /cloudrevo

RUN sed -i 's#https\?://dl-cdn.alpinelinux.org/alpine#https://mirrors.tuna.tsinghua.edu.cn/alpine#g' /etc/apk/repositories \
    && apk update \
    && apk add --no-cache tzdata vips-tools ffmpeg libreoffice font-noto font-noto-cjk libheif libraw-tools\
    && cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime \
    && echo "Asia/Shanghai" > /etc/timezone

COPY --from=backend-build /out/cloudrevo ./cloudrevo

RUN chmod +x ./cloudrevo

EXPOSE 5212 443

VOLUME ["/cloudrevo/data"]

ENTRYPOINT ["./cloudrevo"]
