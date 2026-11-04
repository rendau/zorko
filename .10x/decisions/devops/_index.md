# DevOps — общие решения

- `.github/workflows/deploy.yml` из шаблона: сборка и push образа
  `ghcr.io/rendau/zorko:latest` на раннере `yc-arc`; сейчас выключен (ветка `disabled`).
- Dockerfile: alpine + бинарь + `docs/` + `migrations/`.
- Деплой в кластер (какой, через какой helm-репозиторий) — ещё не решён.
- В подах — `TZ=Asia/Almaty`; конфиг — через kusec.
