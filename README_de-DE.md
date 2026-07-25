# CloudRevo

> Alles, was Sie brauchen: eine Cloud.

[English](README.md) · [简体中文](README_zh-CN.md) · [繁體中文](README_zh-TW.md) · [日本語](README_ja-JP.md) · [한국어](README_ko-KR.md) · [Deutsch](README_de-DE.md) · [Español](README_es-ES.md) · [Français](README_fr-FR.md) · [Italiano](README_it-IT.md) · [Polski](README_pl-PL.md) · [Português (Brasil)](README_pt-BR.md) · [Русский](README_ru-RU.md) · [العربية](README_ar-AR.md)

## Enthaltene Funktionen

- Dateiberechtigungen für Benutzer, Gruppen, anonyme Besucher und andere Benutzer: Anzeigen, Erstellen, Ändern und Löschen.
- Freigabeverwaltung für Dateien und Ordner, mehrere Links, Standardfreigaben für neue Benutzer und Linkverwaltung.
- Gopeed-Offlinedownloads mit Vorabprüfung, Dateiauswahl, Request-Headern, Wiederholungen, Stapelaktionen, Live-Fortschritt und Verbindungen pro Aufgabe. Aria2 ist nicht enthalten.

## TODO

- [ ] OnlyOffice-Zusammenarbeit
- [ ] Status für Zusammenarbeit in Echtzeit
- [ ] Desktop-Synchronisierung
- [ ] Bereitstellungs- und API-Dokumentation
- [ ] Weitere Integrationstests

## Lokal starten

```bash
cp .env.example .env
# POSTGRES_PASSWORD und GOPEED_API_TOKEN in .env setzen.
docker compose up --build
```

## Bereitstellung mit einem Befehl

```bash
mkdir -p cloudrevo && cd cloudrevo && curl -fsSLO https://raw.githubusercontent.com/dadastory/CloudRevo/main/docker-compose.yaml && umask 077 && { printf 'POSTGRES_PASSWORD=%s\n' "$(openssl rand -hex 32)"; printf 'GOPEED_API_TOKEN=%s\n' "$(openssl rand -hex 32)"; } > .env && docker compose up -d
```

## Danksagungen

[Cloudreve](https://github.com/cloudreve/Cloudreve) liefert die GPL-Codebasis, [Gopeed](https://github.com/GopeedLab/gopeed) die Offlinedownload-Engine und [Casbin](https://github.com/casbin/casbin) die Autorisierungsdurchsetzung.

## Lizenz und Feedback

CloudRevo wird unter den abgeleiteten Bedingungen der [GPL-3.0](LICENSE) veröffentlicht. Bitte melden Sie Probleme über [Issues](https://github.com/dadastory/CloudRevo/issues).
