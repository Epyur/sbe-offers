FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
COPY *.go ./
# Swagger UI и схема вшиваются в бинарь через //go:embed (static.go), бланк
# DOCX — через //go:embed (template.go). Без этих строк сборка падает на
# `pattern static/...: no matching files found`: в контекст копируются только
# перечисленные пути, каталогов образ не видит.
COPY static ./static
COPY template ./template
RUN GOFLAGS=-mod=mod CGO_ENABLED=0 GOOS=linux go build -trimpath -o /mailers-offers .

FROM alpine:3.20
# rclone для выгрузки DOCX в S3 (стабильно работает с Ceph, в отличие от
# aws-sdk-go-v2). Закреплённая версия с GitHub, слой ДО копирования бинаря:
# иначе любое изменение Go-кода заставляет качать rclone заново, а
# downloads.rclone.org с этого VDS однажды оказывался недоступен ровно в
# момент деплоя.
RUN apk add --no-cache ca-certificates wget unzip \
    && wget -q -O /tmp/rclone.zip https://github.com/rclone/rclone/releases/download/v1.75.0/rclone-v1.75.0-linux-amd64.zip \
    && unzip -q /tmp/rclone.zip -d /tmp/rclone \
    && mv /tmp/rclone/rclone-*-linux-amd64/rclone /usr/local/bin/rclone \
    && chmod +x /usr/local/bin/rclone \
    && rm -rf /tmp/rclone /tmp/rclone.zip \
    && apk del wget unzip
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /mailers-offers /mailers-offers
EXPOSE 3000
ENTRYPOINT ["/mailers-offers"]
