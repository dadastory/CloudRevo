# CloudRevo

> 你所需要的一切，只需一個網盤。

[English](README.md) · [简体中文](README_zh-CN.md) · [繁體中文](README_zh-TW.md) · [日本語](README_ja-JP.md) · [한국어](README_ko-KR.md) · [Deutsch](README_de-DE.md) · [Español](README_es-ES.md) · [Français](README_fr-FR.md) · [Italiano](README_it-IT.md) · [Polski](README_pl-PL.md) · [Português (Brasil)](README_pt-BR.md) · [Русский](README_ru-RU.md) · [العربية](README_ar-AR.md)

## 已實現功能

- 檔案權限：為使用者、使用者群組、匿名訪客及其他使用者設定檢視、建立、修改與刪除權限。
- 分享管理：支援檔案與資料夾分享、多個連結、新使用者預設分享及連結管理。
- Gopeed 離線下載：支援預檢、檔案選擇、請求標頭、重試、批次操作、即時進度與單任務連線數；不含 Aria2。

## TODO

- [ ] OnlyOffice 協作
- [ ] 即時協作狀態
- [ ] 桌面端同步
- [ ] 部署與 API 文件
- [ ] 更多整合測試

## 本機啟動

```bash
cp .env.example .env
# 在 .env 中設定 POSTGRES_PASSWORD 與 GOPEED_API_TOKEN。
docker compose up --build
```

## 一鍵部署

```bash
mkdir -p cloudrevo && cd cloudrevo && curl -fsSLO https://raw.githubusercontent.com/dadastory/CloudRevo/main/docker-compose.yaml && umask 077 && { printf 'POSTGRES_PASSWORD=%s\n' "$(openssl rand -hex 32)"; printf 'GOPEED_API_TOKEN=%s\n' "$(openssl rand -hex 32)"; } > .env && docker compose up -d
```

## 致謝

[Cloudreve](https://github.com/cloudreve/Cloudreve) 提供 GPL 基礎程式碼；[Gopeed](https://github.com/GopeedLab/gopeed) 提供離線下載引擎；[Casbin](https://github.com/casbin/casbin) 提供授權執行機制。

## 授權與回饋

CloudRevo 依 [GPL-3.0](LICENSE) 衍生條款發佈。若發現問題，請至 [Issues](https://github.com/dadastory/CloudRevo/issues) 回報。
