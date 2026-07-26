# CloudRevo

> Tudo o que você precisa, em uma única nuvem.

[English](README.md) · [简体中文](README_zh-CN.md) · [繁體中文](README_zh-TW.md) · [日本語](README_ja-JP.md) · [한국어](README_ko-KR.md) · [Deutsch](README_de-DE.md) · [Español](README_es-ES.md) · [Français](README_fr-FR.md) · [Italiano](README_it-IT.md) · [Polski](README_pl-PL.md) · [Português (Brasil)](README_pt-BR.md) · [Русский](README_ru-RU.md) · [العربية](README_ar-AR.md)

## Recursos incluídos

- Permissões de arquivo para usuários, grupos, visitantes anônimos e outros usuários: visualizar, criar, modificar e excluir.
- Gerenciamento de compartilhamentos de arquivos e pastas, múltiplos links, compartilhamentos padrão para novos usuários e administração de links.
- Downloads offline com Gopeed: pré-verificação, seleção de arquivos, cabeçalhos de requisição, tentativas, ações em lote, progresso ao vivo e conexões por tarefa. Aria2 não está incluído.

## TODO

- [ ] Colaboração com OnlyOffice
- [ ] Indicadores de colaboração em tempo real
- [ ] Sincronização para desktop
- [ ] Documentação de implantação e API

## Início local

```bash
cp .env.example .env
# Defina POSTGRES_PASSWORD e GOPEED_API_TOKEN em .env.
docker compose up --build
```

## Implantação com um comando

```bash
mkdir -p cloudrevo && cd cloudrevo && curl -fsSLO https://raw.githubusercontent.com/dadastory/CloudRevo/main/docker-compose.yaml && umask 077 && { printf 'POSTGRES_PASSWORD=%s\n' "$(openssl rand -hex 32)"; printf 'GOPEED_API_TOKEN=%s\n' "$(openssl rand -hex 32)"; } > .env && docker compose up -d
```

## Agradecimentos

[Cloudreve](https://github.com/cloudreve/Cloudreve) fornece a base de código GPL, [Gopeed](https://github.com/GopeedLab/gopeed) o mecanismo de downloads offline e [Casbin](https://github.com/casbin/casbin) a aplicação de autorização.

## Licença e comentários

CloudRevo é publicado sob os termos derivados da [GPL-3.0](LICENSE). Relate problemas em [Issues](https://github.com/dadastory/CloudRevo/issues).
