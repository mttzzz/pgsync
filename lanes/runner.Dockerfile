FROM golang:1.25-trixie

# Версия линтера — как в .github/workflows/ci.yml (golangci-lint-action → version): локальный
# `golangci-lint run ./...` обязан давать тот же вердикт, что CI.
ARG GOLANGCI_LINT_VERSION=v2.12.1

# golang:*-trixie уже несёт gcc/make/git/curl (cgo нужен `go test -race`). Пользователь dev —
# uid 1000, как у хозяина чекаута: под runner'а идёт с uid/gid 1000 (securityContext в
# herdr-lanes/manifests/runner.yaml) и пишет в смонтированное дерево от его имени, иначе файлы
# (bin/pgsync, coverage.out) стали бы root-овыми для хостового редактора.
RUN curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh \
      | sh -s -- -b /usr/local/bin ${GOLANGCI_LINT_VERSION} \
 && useradd --uid 1000 --create-home --shell /bin/bash dev

# Модульный кеш и кеши сборки/линтера — в домашнем каталоге dev (HOME=/home/dev), а не в /go
# базового образа: тот общий и root-овый.
ENV HOME=/home/dev \
    GOPATH=/home/dev/go \
    GOCACHE=/home/dev/.cache/go-build \
    GOLANGCI_LINT_CACHE=/home/dev/.cache/golangci-lint

USER dev
