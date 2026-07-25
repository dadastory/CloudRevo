<p align="center">
  <img src="assets/public/static/img/logo.svg" width="280" alt="CloudRevo">
</p>

<p align="center">快速、自托管的文件存储，提供权限、分享与离线下载。</p>

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
