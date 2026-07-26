# CloudRevo

> كل ما تحتاجه، في سحابة واحدة.

[English](README.md) · [简体中文](README_zh-CN.md) · [繁體中文](README_zh-TW.md) · [日本語](README_ja-JP.md) · [한국어](README_ko-KR.md) · [Deutsch](README_de-DE.md) · [Español](README_es-ES.md) · [Français](README_fr-FR.md) · [Italiano](README_it-IT.md) · [Polski](README_pl-PL.md) · [Português (Brasil)](README_pt-BR.md) · [Русский](README_ru-RU.md) · [العربية](README_ar-AR.md)

## الميزات المتاحة

- أذونات الملفات للمستخدمين والمجموعات والزوار المجهولين والمستخدمين الآخرين: العرض والإنشاء والتعديل والحذف.
- إدارة مشاركة الملفات والمجلدات والروابط المتعددة والمشاركات الافتراضية للمستخدمين الجدد وإدارة الروابط.
- تنزيلات Gopeed دون اتصال: فحص مسبق واختيار الملفات وترويسات الطلبات وإعادة المحاولة والعمليات المجمعة والتقدم المباشر وعدد الاتصالات لكل مهمة. لا يتضمن Aria2.

## TODO

- [ ] تعاون OnlyOffice
- [ ] مؤشرات التعاون في الوقت الحقيقي
- [ ] مزامنة سطح المكتب
- [ ] وثائق النشر وواجهة API

## التشغيل محلياً

```bash
cp .env.example .env
# عيّن POSTGRES_PASSWORD و GOPEED_API_TOKEN في .env.
docker compose up --build
```

## النشر بأمر واحد

```bash
mkdir -p cloudrevo && cd cloudrevo && curl -fsSLO https://raw.githubusercontent.com/dadastory/CloudRevo/main/docker-compose.yaml && umask 077 && { printf 'POSTGRES_PASSWORD=%s\n' "$(openssl rand -hex 32)"; printf 'GOPEED_API_TOKEN=%s\n' "$(openssl rand -hex 32)"; } > .env && docker compose up -d
```

## الشكر والتقدير

يوفر [Cloudreve](https://github.com/cloudreve/Cloudreve) قاعدة كود GPL، ويوفر [Gopeed](https://github.com/GopeedLab/gopeed) محرك التنزيل دون اتصال، ويوفر [Casbin](https://github.com/casbin/casbin) تطبيق التفويض.

## الترخيص والملاحظات

يُنشر CloudRevo وفق الشروط المشتقة من [GPL-3.0](LICENSE). أبلغ عن المشكلات عبر [Issues](https://github.com/dadastory/CloudRevo/issues).
