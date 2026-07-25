<p align="center">
  <a href="https://github.com/dadastory/CloudRevo_web">
    <picture>
      <source media="(prefers-color-scheme: dark)" srcset="https://raw.githubusercontent.com/dadastory/CloudRevo/main/docs/brand/logo_light.svg">
      <source media="(prefers-color-scheme: light)" srcset="https://raw.githubusercontent.com/dadastory/CloudRevo/main/docs/brand/logo.svg">
      <img src="https://raw.githubusercontent.com/dadastory/CloudRevo/main/docs/brand/logo.svg" width="500" alt="CloudRevo">
    </picture>
  </a>
</p>

<p align="center">你所需要的一切，只需一个网盘。</p>

<p align="center">
  <a href="https://github.com/dadastory/CloudRevo/actions/workflows/release.yml"><img src="https://img.shields.io/github/actions/workflow/status/dadastory/CloudRevo/release.yml?branch=main&style=for-the-badge&label=build" alt="构建状态"></a>
  <a href="LICENSE"><img src="https://img.shields.io/github/license/dadastory/CloudRevo?style=for-the-badge" alt="GPL-3.0 许可证"></a>
  <a href="https://github.com/dadastory/CloudRevo/stargazers"><img src="https://img.shields.io/github/stars/dadastory/CloudRevo?style=for-the-badge" alt="GitHub Star"></a>
  <a href="https://github.com/dadastory/CloudRevo/forks"><img src="https://img.shields.io/github/forks/dadastory/CloudRevo?style=for-the-badge" alt="GitHub Fork"></a>
  <a href="https://github.com/dadastory/CloudRevo/issues"><img src="https://img.shields.io/github/issues/dadastory/CloudRevo?style=for-the-badge" alt="未解决 Issue"></a>
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
  <a href="https://github.com/dadastory/CloudRevo/issues">反馈问题</a>
</p>

## 已实现功能

- **文件权限**：支持用户、用户组、匿名访客和其他用户的查看、创建、修改、删除权限。
- **分享管理**：支持文件和文件夹分享、多分享链接、新用户默认分享与链接管理。
- **Gopeed 离线下载**：支持预检查、文件选择、请求头、重试、批量操作、实时进度及单任务连接数设置；不再包含 Aria2。

## TODO

- [ ] OnlyOffice 协作
- [ ] 实时协作状态
- [ ] 桌面端同步
- [ ] 部署与 API 文档
- [ ] 更多集成测试

## 社区

- 在 [GitHub Discussions](https://github.com/dadastory/CloudRevo/discussions) 提问、分享部署经验或提出想法。
- 通过 [Issues](https://github.com/dadastory/CloudRevo/issues) 反馈可复现问题与安全相关问题。
- 如果 CloudRevo 对你有帮助，欢迎 Star 或分享，让项目更容易被需要的人发现。

## 本地启动

CloudRevo 会在本地构建应用镜像，持久化数据保存在 `./storage/`。

```bash
cp .env.example .env
# 在 .env 中设置 POSTGRES_PASSWORD 和 GOPEED_API_TOKEN。
docker compose up --build
```

## 一键部署

以下命令拉取已发布的 CloudRevo 镜像，并将所有数据保存到当前目录；不会执行远程脚本。

```bash
mkdir -p cloudrevo && cd cloudrevo && curl -fsSLO https://raw.githubusercontent.com/dadastory/CloudRevo/main/docker-compose.yaml && umask 077 && { printf 'POSTGRES_PASSWORD=%s\n' "$(openssl rand -hex 32)"; printf 'GOPEED_API_TOKEN=%s\n' "$(openssl rand -hex 32)"; } > .env && docker compose up -d
```

## 反馈与免责声明

项目仍在持续开发。如果发现功能异常、缺失或安全问题，请在 [Issue](https://github.com/dadastory/CloudRevo/issues) 中提供复现步骤；安全敏感问题请按 [SECURITY.md](SECURITY.md) 反馈。

CloudRevo 按“现状”提供，不提供任何担保。你需要自行负责备份、访问控制配置、数据处理，以及适用于自身使用场景的法律合规。

## 致谢

CloudRevo 的实现离不开以下项目：

- [Cloudreve](https://github.com/cloudreve/Cloudreve) 提供了 CloudRevo 所衍生的 GPL-3.0 文件存储基础。
- [Gopeed](https://github.com/GopeedLab/gopeed) 提供离线下载引擎；CloudRevo 的集成维护在 [dadastory/gopeed](https://github.com/dadastory/gopeed)。
- [Casbin](https://github.com/casbin/casbin) 提供文件权限所依赖的授权模型和策略执行机制。

## 许可证

CloudRevo 是 Cloudreve 的修改衍生项目，依据 [GPL-3.0](LICENSE) 发布，并非 Cloudreve 官方发行版。
