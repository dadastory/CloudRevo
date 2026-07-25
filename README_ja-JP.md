# CloudRevo

> 必要なものは、ひとつのクラウドに。

[English](README.md) · [简体中文](README_zh-CN.md) · [繁體中文](README_zh-TW.md) · [日本語](README_ja-JP.md) · [한국어](README_ko-KR.md) · [Deutsch](README_de-DE.md) · [Español](README_es-ES.md) · [Français](README_fr-FR.md) · [Italiano](README_it-IT.md) · [Polski](README_pl-PL.md) · [Português (Brasil)](README_pt-BR.md) · [Русский](README_ru-RU.md) · [العربية](README_ar-AR.md)

## 実装済みの機能

- ファイル権限：ユーザー、グループ、匿名訪問者、その他のユーザーに対し、閲覧・作成・変更・削除の権限を設定できます。
- 共有管理：ファイルとフォルダーの共有、複数リンク、新規ユーザー向けの既定共有、リンク管理に対応します。
- Gopeed オフラインダウンロード：事前確認、ファイル選択、リクエストヘッダー、再試行、一括操作、リアルタイム進捗、タスクごとの接続数に対応します。Aria2 は含みません。

## TODO

- [ ] OnlyOffice 連携
- [ ] リアルタイム共同編集の状態表示
- [ ] デスクトップ同期
- [ ] デプロイと API のドキュメント
- [ ] 統合テストの追加

## ローカルで起動

```bash
cp .env.example .env
# .env に POSTGRES_PASSWORD と GOPEED_API_TOKEN を設定します。
docker compose up --build
```

## ワンコマンドデプロイ

```bash
mkdir -p cloudrevo && cd cloudrevo && curl -fsSLO https://raw.githubusercontent.com/dadastory/CloudRevo/main/docker-compose.yaml && umask 077 && { printf 'POSTGRES_PASSWORD=%s\n' "$(openssl rand -hex 32)"; printf 'GOPEED_API_TOKEN=%s\n' "$(openssl rand -hex 32)"; } > .env && docker compose up -d
```

## 謝辞

[Cloudreve](https://github.com/cloudreve/Cloudreve) は GPL の基盤コード、[Gopeed](https://github.com/GopeedLab/gopeed) はオフラインダウンロードエンジン、[Casbin](https://github.com/casbin/casbin) は認可実行機構を提供しています。

## ライセンスとフィードバック

CloudRevo は [GPL-3.0](LICENSE) の派生条件で公開されています。問題は [Issues](https://github.com/dadastory/CloudRevo/issues) で報告してください。
