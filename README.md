<p align="center">
  <img src="assets/public/static/img/logo.svg" width="280" alt="CloudRevo">
</p>

<p align="center">Fast, self-hosted file storage with permissions, sharing, and offline downloads.</p>

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
