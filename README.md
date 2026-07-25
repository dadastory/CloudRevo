<p align="center">
  <a href="https://github.com/dadastory/CloudRevo_web">
    <img src="https://raw.githubusercontent.com/dadastory/CloudRevo_web/main/public/static/img/logo.svg" width="280" alt="CloudRevo">
  </a>
</p>

<p align="center">Fast, self-hosted file storage with permissions, sharing, and offline downloads.</p>

<p align="center">
  <a href="README.md">English</a> ·
  <a href="README_zh-CN.md">简体中文</a> ·
  <a href="#interface-languages">Other interface languages</a>
</p>

<p align="center">
  <a href="LICENSE">GPL-3.0</a> ·
  <a href="https://github.com/dadastory/CloudRevo/issues">Report an issue</a>
</p>

## What is included

- **File permissions** — view, create, modify, and delete rules for users, groups, anonymous visitors, and other users.
- **Share management** — file and folder shares, multiple links, default shares for new users, and link administration.
- **Gopeed offline downloads** — preflight inspection, file selection, headers, retries, batch actions, live progress, and per-task connection settings. Aria2 is not included.

## TODO

- [ ] OnlyOffice collaboration
- [ ] Real-time collaboration indicators
- [ ] Desktop sync support
- [ ] Deployment and API documentation
- [ ] More integration tests

## Interface languages

The frontend includes interface translations for [العربية](https://github.com/dadastory/CloudRevo_web/tree/main/public/locales/ar-AR), [Deutsch](https://github.com/dadastory/CloudRevo_web/tree/main/public/locales/de-DE), [Español](https://github.com/dadastory/CloudRevo_web/tree/main/public/locales/es-ES), [Français](https://github.com/dadastory/CloudRevo_web/tree/main/public/locales/fr-FR), [Italiano](https://github.com/dadastory/CloudRevo_web/tree/main/public/locales/it-IT), [日本語](https://github.com/dadastory/CloudRevo_web/tree/main/public/locales/ja-JP), [한국어](https://github.com/dadastory/CloudRevo_web/tree/main/public/locales/ko-KR), [Polski](https://github.com/dadastory/CloudRevo_web/tree/main/public/locales/pl-PL), [Português (Brasil)](https://github.com/dadastory/CloudRevo_web/tree/main/public/locales/pt-BR), [Русский](https://github.com/dadastory/CloudRevo_web/tree/main/public/locales/ru-RU), and [繁體中文](https://github.com/dadastory/CloudRevo_web/tree/main/public/locales/zh-TW). English and Simplified Chinese documentation are linked above.

## Start locally

CloudRevo builds its application images locally. Persistent data is stored in `./storage/`.

```bash
cp .env.example .env
# Set POSTGRES_PASSWORD and GOPEED_API_TOKEN in .env.
docker compose up --build
```

## One-click deployment

This pulls published CloudRevo images and keeps all data in the current directory. It does not execute a remote script.

```bash
mkdir -p cloudrevo && cd cloudrevo && curl -fsSLO https://raw.githubusercontent.com/dadastory/CloudRevo/main/docker-compose.yaml && umask 077 && { printf 'POSTGRES_PASSWORD=%s\n' "$(openssl rand -hex 32)"; printf 'GOPEED_API_TOKEN=%s\n' "$(openssl rand -hex 32)"; } > .env && docker compose up -d
```

## Feedback and disclaimer

This project is under active development. If something is broken, incomplete, or unsafe, please open an [issue](https://github.com/dadastory/CloudRevo/issues) with steps to reproduce it. Security-sensitive reports should follow [SECURITY.md](SECURITY.md).

CloudRevo is provided **as is**, without warranty. You are responsible for backups, access-control configuration, data handling, and compliance with laws that apply to your use.

## Acknowledgements

CloudRevo exists because of these projects:

- [Cloudreve](https://github.com/cloudreve/Cloudreve) provides the GPL-3.0 file-storage foundation from which CloudRevo is derived.
- [Gopeed](https://github.com/GopeedLab/gopeed) provides the offline-download engine. CloudRevo maintains its integration at [dadastory/gopeed](https://github.com/dadastory/gopeed).
- [Casbin](https://github.com/casbin/casbin) provides the authorization model and policy-enforcement foundation for file permissions.

## Licence

CloudRevo is a modified derivative of Cloudreve, released under [GPL-3.0](LICENSE), and is not an official Cloudreve release.
