# CloudRevo

> Wszystko, czego potrzebujesz, w jednej chmurze.

[English](README.md) · [简体中文](README_zh-CN.md) · [繁體中文](README_zh-TW.md) · [日本語](README_ja-JP.md) · [한국어](README_ko-KR.md) · [Deutsch](README_de-DE.md) · [Español](README_es-ES.md) · [Français](README_fr-FR.md) · [Italiano](README_it-IT.md) · [Polski](README_pl-PL.md) · [Português (Brasil)](README_pt-BR.md) · [Русский](README_ru-RU.md) · [العربية](README_ar-AR.md)

## Dostępne funkcje

- Uprawnienia do plików dla użytkowników, grup, anonimowych odwiedzających i innych użytkowników: wyświetlanie, tworzenie, modyfikowanie i usuwanie.
- Zarządzanie udostępnieniami plików i folderów, wieloma linkami, domyślnymi udostępnieniami dla nowych użytkowników oraz linkami.
- Pobieranie offline przez Gopeed: wstępna kontrola, wybór plików, nagłówki żądań, ponawianie, operacje zbiorcze, postęp na żywo i liczba połączeń na zadanie. Aria2 nie jest dołączone.

## TODO

- [ ] Współpraca OnlyOffice
- [ ] Wskaźniki współpracy w czasie rzeczywistym
- [ ] Synchronizacja pulpitu
- [ ] Dokumentacja wdrożenia i API

## Uruchomienie lokalne

```bash
cp .env.example .env
# Ustaw POSTGRES_PASSWORD i GOPEED_API_TOKEN w .env.
docker compose up --build
```

## Wdrożenie jednym poleceniem

```bash
mkdir -p cloudrevo && cd cloudrevo && curl -fsSLO https://raw.githubusercontent.com/dadastory/CloudRevo/main/docker-compose.yaml && umask 077 && { printf 'POSTGRES_PASSWORD=%s\n' "$(openssl rand -hex 32)"; printf 'GOPEED_API_TOKEN=%s\n' "$(openssl rand -hex 32)"; } > .env && docker compose up -d
```

## Podziękowania

[Cloudreve](https://github.com/cloudreve/Cloudreve) zapewnia bazę kodu GPL, [Gopeed](https://github.com/GopeedLab/gopeed) silnik pobierania offline, a [Casbin](https://github.com/casbin/casbin) egzekwowanie autoryzacji.

## Licencja i opinie

CloudRevo jest publikowane na warunkach pochodnych [GPL-3.0](LICENSE). Problemy zgłaszaj w [Issues](https://github.com/dadastory/CloudRevo/issues).
