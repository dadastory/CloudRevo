<p align="center">
  <a href="https://github.com/dadastory/CloudRevo_web">
    <picture>
      <source media="(prefers-color-scheme: dark)" srcset="https://raw.githubusercontent.com/dadastory/CloudRevo/main/docs/brand/logo_light.svg">
      <source media="(prefers-color-scheme: light)" srcset="https://raw.githubusercontent.com/dadastory/CloudRevo/main/docs/brand/logo.svg">
      <img src="https://raw.githubusercontent.com/dadastory/CloudRevo/main/docs/brand/logo.svg" width="380" alt="CloudRevo">
    </picture>
  </a>
</p>

<p align="center">All you need is one cloud.</p>

<p align="center">
  <a href="https://github.com/dadastory/CloudRevo/actions/workflows/release.yml"><img src="https://img.shields.io/github/actions/workflow/status/dadastory/CloudRevo/release.yml?branch=main&style=for-the-badge&label=build" alt="Build status"></a>
  <a href="LICENSE"><img src="https://img.shields.io/github/license/dadastory/CloudRevo?style=for-the-badge" alt="GPL-3.0 license"></a>
  <a href="https://github.com/dadastory/CloudRevo/stargazers"><img src="https://img.shields.io/github/stars/dadastory/CloudRevo?style=for-the-badge" alt="GitHub stars"></a>
  <a href="https://github.com/dadastory/CloudRevo/forks"><img src="https://img.shields.io/github/forks/dadastory/CloudRevo?style=for-the-badge" alt="GitHub forks"></a>
  <a href="https://github.com/dadastory/CloudRevo/issues"><img src="https://img.shields.io/github/issues/dadastory/CloudRevo?style=for-the-badge" alt="Open issues"></a>
</p>

<p align="center">
  <a href="README.md">English</a> ·
  <a href="README_zh-CN.md">简体中文</a> ·
  <a href="README_zh-TW.md">繁體中文</a> ·
  <a href="README_ja-JP.md">日本語</a> ·
  <a href="README_ko-KR.md">한국어</a> ·
  <a href="README_de-DE.md">Deutsch</a> ·
  <a href="README_es-ES.md">Español</a> ·
  <a href="README_fr-FR.md">Français</a> ·
  <a href="README_it-IT.md">Italiano</a> ·
  <a href="README_pl-PL.md">Polski</a> ·
  <a href="README_pt-BR.md">Português (Brasil)</a> ·
  <a href="README_ru-RU.md">Русский</a> ·
  <a href="README_ar-AR.md">العربية</a>
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

## Community

- Ask questions, share deployment experience, or propose ideas in [GitHub Discussions](https://github.com/dadastory/CloudRevo/discussions).
- Report reproducible bugs and security-sensitive concerns through [Issues](https://github.com/dadastory/CloudRevo/issues).
- Star and share CloudRevo if it is useful to you; it makes the project easier to discover.

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
