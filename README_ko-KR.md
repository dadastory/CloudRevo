# CloudRevo

> 필요한 모든 것, 단 하나의 클라우드.

[English](README.md) · [简体中文](README_zh-CN.md) · [繁體中文](README_zh-TW.md) · [日本語](README_ja-JP.md) · [한국어](README_ko-KR.md) · [Deutsch](README_de-DE.md) · [Español](README_es-ES.md) · [Français](README_fr-FR.md) · [Italiano](README_it-IT.md) · [Polski](README_pl-PL.md) · [Português (Brasil)](README_pt-BR.md) · [Русский](README_ru-RU.md) · [العربية](README_ar-AR.md)

## 구현된 기능

- 파일 권한: 사용자, 그룹, 익명 방문자 및 기타 사용자에게 보기, 만들기, 수정, 삭제 권한을 설정합니다.
- 공유 관리: 파일·폴더 공유, 여러 링크, 신규 사용자의 기본 공유 및 링크 관리를 지원합니다.
- Gopeed 오프라인 다운로드: 사전 검사, 파일 선택, 요청 헤더, 재시도, 일괄 작업, 실시간 진행률 및 작업별 연결 수를 지원합니다. Aria2는 포함하지 않습니다.

## TODO

- [ ] OnlyOffice 협업
- [ ] 실시간 협업 상태
- [ ] 데스크톱 동기화
- [ ] 배포 및 API 문서

## 로컬 시작

```bash
cp .env.example .env
# .env에 POSTGRES_PASSWORD와 GOPEED_API_TOKEN을 설정합니다.
docker compose up --build
```

## 원클릭 배포

```bash
mkdir -p cloudrevo && cd cloudrevo && curl -fsSLO https://raw.githubusercontent.com/dadastory/CloudRevo/main/docker-compose.yaml && umask 077 && { printf 'POSTGRES_PASSWORD=%s\n' "$(openssl rand -hex 32)"; printf 'GOPEED_API_TOKEN=%s\n' "$(openssl rand -hex 32)"; } > .env && docker compose up -d
```

## 감사의 말

[Cloudreve](https://github.com/cloudreve/Cloudreve)는 GPL 기반 코드를, [Gopeed](https://github.com/GopeedLab/gopeed)는 오프라인 다운로드 엔진을, [Casbin](https://github.com/casbin/casbin)은 인가 실행 체계를 제공합니다.

## 라이선스 및 피드백

CloudRevo는 [GPL-3.0](LICENSE) 파생 조건으로 배포됩니다. 문제는 [Issues](https://github.com/dadastory/CloudRevo/issues)에 알려 주세요.
