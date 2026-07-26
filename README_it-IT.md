# CloudRevo

> Tutto ciò che ti serve, in un solo cloud.

[English](README.md) · [简体中文](README_zh-CN.md) · [繁體中文](README_zh-TW.md) · [日本語](README_ja-JP.md) · [한국어](README_ko-KR.md) · [Deutsch](README_de-DE.md) · [Español](README_es-ES.md) · [Français](README_fr-FR.md) · [Italiano](README_it-IT.md) · [Polski](README_pl-PL.md) · [Português (Brasil)](README_pt-BR.md) · [Русский](README_ru-RU.md) · [العربية](README_ar-AR.md)

## Funzionalità incluse

- Permessi dei file per utenti, gruppi, visitatori anonimi e altri utenti: visualizzare, creare, modificare ed eliminare.
- Gestione delle condivisioni di file e cartelle, collegamenti multipli, condivisioni predefinite per nuovi utenti e gestione dei collegamenti.
- Download offline con Gopeed: controllo preliminare, selezione dei file, intestazioni HTTP, tentativi, azioni in blocco, avanzamento in tempo reale e connessioni per attività. Aria2 non è incluso.

## TODO

- [ ] Collaborazione OnlyOffice
- [ ] Indicatori di collaborazione in tempo reale
- [ ] Sincronizzazione desktop
- [ ] Documentazione di distribuzione e API

## Avvio locale

```bash
cp .env.example .env
# Impostare POSTGRES_PASSWORD e GOPEED_API_TOKEN in .env.
docker compose up --build
```

## Distribuzione con un comando

```bash
mkdir -p cloudrevo && cd cloudrevo && curl -fsSLO https://raw.githubusercontent.com/dadastory/CloudRevo/main/docker-compose.yaml && umask 077 && { printf 'POSTGRES_PASSWORD=%s\n' "$(openssl rand -hex 32)"; printf 'GOPEED_API_TOKEN=%s\n' "$(openssl rand -hex 32)"; } > .env && docker compose up -d
```

## Ringraziamenti

[Cloudreve](https://github.com/cloudreve/Cloudreve) fornisce la base di codice GPL, [Gopeed](https://github.com/GopeedLab/gopeed) il motore di download offline e [Casbin](https://github.com/casbin/casbin) l'applicazione delle autorizzazioni.

## Licenza e feedback

CloudRevo è pubblicato secondo i termini derivati dalla [GPL-3.0](LICENSE). Segnala i problemi nelle [Issues](https://github.com/dadastory/CloudRevo/issues).
