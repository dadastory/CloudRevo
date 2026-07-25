# CloudRevo

> Tout ce dont vous avez besoin, dans un seul cloud.

[English](README.md) · [简体中文](README_zh-CN.md) · [繁體中文](README_zh-TW.md) · [日本語](README_ja-JP.md) · [한국어](README_ko-KR.md) · [Deutsch](README_de-DE.md) · [Español](README_es-ES.md) · [Français](README_fr-FR.md) · [Italiano](README_it-IT.md) · [Polski](README_pl-PL.md) · [Português (Brasil)](README_pt-BR.md) · [Русский](README_ru-RU.md) · [العربية](README_ar-AR.md)

## Fonctionnalités incluses

- Autorisations de fichiers pour les utilisateurs, groupes, visiteurs anonymes et autres utilisateurs : consulter, créer, modifier et supprimer.
- Gestion des partages de fichiers et dossiers, liens multiples, partages par défaut pour les nouveaux utilisateurs et administration des liens.
- Téléchargements hors ligne Gopeed : pré-vérification, sélection de fichiers, en-têtes HTTP, reprises, actions par lot, progression en direct et connexions par tâche. Aria2 n'est pas inclus.

## TODO

- [ ] Collaboration OnlyOffice
- [ ] Indicateurs de collaboration en temps réel
- [ ] Synchronisation de bureau
- [ ] Documentation de déploiement et d'API
- [ ] Davantage de tests d'intégration

## Démarrer en local

```bash
cp .env.example .env
# Configurez POSTGRES_PASSWORD et GOPEED_API_TOKEN dans .env.
docker compose up --build
```

## Déploiement en une commande

```bash
mkdir -p cloudrevo && cd cloudrevo && curl -fsSLO https://raw.githubusercontent.com/dadastory/CloudRevo/main/docker-compose.yaml && umask 077 && { printf 'POSTGRES_PASSWORD=%s\n' "$(openssl rand -hex 32)"; printf 'GOPEED_API_TOKEN=%s\n' "$(openssl rand -hex 32)"; } > .env && docker compose up -d
```

## Remerciements

[Cloudreve](https://github.com/cloudreve/Cloudreve) fournit la base GPL, [Gopeed](https://github.com/GopeedLab/gopeed) le moteur de téléchargement hors ligne et [Casbin](https://github.com/casbin/casbin) l'application des autorisations.

## Licence et retours

CloudRevo est publié selon les termes dérivés de la [GPL-3.0](LICENSE). Signalez les problèmes dans les [Issues](https://github.com/dadastory/CloudRevo/issues).
