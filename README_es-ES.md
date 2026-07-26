# CloudRevo

> Todo lo que necesitas, en una sola nube.

[English](README.md) · [简体中文](README_zh-CN.md) · [繁體中文](README_zh-TW.md) · [日本語](README_ja-JP.md) · [한국어](README_ko-KR.md) · [Deutsch](README_de-DE.md) · [Español](README_es-ES.md) · [Français](README_fr-FR.md) · [Italiano](README_it-IT.md) · [Polski](README_pl-PL.md) · [Português (Brasil)](README_pt-BR.md) · [Русский](README_ru-RU.md) · [العربية](README_ar-AR.md)

## Funciones incluidas

- Permisos de archivo para usuarios, grupos, visitantes anónimos y otros usuarios: ver, crear, modificar y eliminar.
- Gestión de compartidos para archivos y carpetas, enlaces múltiples, compartidos predeterminados para nuevos usuarios y administración de enlaces.
- Descargas sin conexión con Gopeed: comprobación previa, selección de archivos, cabeceras, reintentos, acciones por lote, progreso en directo y conexiones por tarea. Aria2 no está incluido.

## TODO

- [ ] Colaboración con OnlyOffice
- [ ] Indicadores de colaboración en tiempo real
- [ ] Sincronización de escritorio
- [ ] Documentación de despliegue y API

## Inicio local

```bash
cp .env.example .env
# Configure POSTGRES_PASSWORD y GOPEED_API_TOKEN en .env.
docker compose up --build
```

## Despliegue con un comando

```bash
mkdir -p cloudrevo && cd cloudrevo && curl -fsSLO https://raw.githubusercontent.com/dadastory/CloudRevo/main/docker-compose.yaml && umask 077 && { printf 'POSTGRES_PASSWORD=%s\n' "$(openssl rand -hex 32)"; printf 'GOPEED_API_TOKEN=%s\n' "$(openssl rand -hex 32)"; } > .env && docker compose up -d
```

## Agradecimientos

[Cloudreve](https://github.com/cloudreve/Cloudreve) aporta la base de código GPL, [Gopeed](https://github.com/GopeedLab/gopeed) el motor de descargas sin conexión y [Casbin](https://github.com/casbin/casbin) la aplicación de autorizaciones.

## Licencia y comentarios

CloudRevo se publica bajo los términos derivados de [GPL-3.0](LICENSE). Informe problemas en [Issues](https://github.com/dadastory/CloudRevo/issues).
