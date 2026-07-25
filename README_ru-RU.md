# CloudRevo

> Всё, что вам нужно, — в одном облаке.

[English](README.md) · [简体中文](README_zh-CN.md) · [繁體中文](README_zh-TW.md) · [日本語](README_ja-JP.md) · [한국어](README_ko-KR.md) · [Deutsch](README_de-DE.md) · [Español](README_es-ES.md) · [Français](README_fr-FR.md) · [Italiano](README_it-IT.md) · [Polski](README_pl-PL.md) · [Português (Brasil)](README_pt-BR.md) · [Русский](README_ru-RU.md) · [العربية](README_ar-AR.md)

## Реализованные возможности

- Права на файлы для пользователей, групп, анонимных посетителей и прочих пользователей: просмотр, создание, изменение и удаление.
- Управление общим доступом к файлам и папкам, несколькими ссылками, общими папками по умолчанию для новых пользователей и ссылками.
- Офлайн-загрузки через Gopeed: предварительная проверка, выбор файлов, заголовки запросов, повторы, пакетные действия, прогресс в реальном времени и число соединений для задачи. Aria2 не включён.

## TODO

- [ ] Совместная работа OnlyOffice
- [ ] Индикаторы совместной работы в реальном времени
- [ ] Синхронизация для компьютера
- [ ] Документация по развёртыванию и API
- [ ] Больше интеграционных тестов

## Локальный запуск

```bash
cp .env.example .env
# Укажите POSTGRES_PASSWORD и GOPEED_API_TOKEN в .env.
docker compose up --build
```

## Развёртывание одной командой

```bash
mkdir -p cloudrevo && cd cloudrevo && curl -fsSLO https://raw.githubusercontent.com/dadastory/CloudRevo/main/docker-compose.yaml && umask 077 && { printf 'POSTGRES_PASSWORD=%s\n' "$(openssl rand -hex 32)"; printf 'GOPEED_API_TOKEN=%s\n' "$(openssl rand -hex 32)"; } > .env && docker compose up -d
```

## Благодарности

[Cloudreve](https://github.com/cloudreve/Cloudreve) предоставляет базу GPL-кода, [Gopeed](https://github.com/GopeedLab/gopeed) — движок офлайн-загрузки, а [Casbin](https://github.com/casbin/casbin) — механизм применения авторизации.

## Лицензия и обратная связь

CloudRevo распространяется на производных условиях [GPL-3.0](LICENSE). Сообщайте о проблемах в [Issues](https://github.com/dadastory/CloudRevo/issues).
