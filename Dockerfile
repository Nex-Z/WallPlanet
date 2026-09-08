FROM node:24-alpine AS web
WORKDIR /src/web
RUN corepack enable && corepack prepare pnpm@9.12.1 --activate
COPY web/package.json web/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY web/ ./
RUN pnpm build

FROM golang:1.26.2-alpine AS server
WORKDIR /src/server
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /wallplanet .

FROM alpine:3.23
RUN apk add --no-cache ca-certificates tzdata && addgroup -S wallplanet && adduser -S -G wallplanet wallplanet
WORKDIR /app/server
COPY --from=server /wallplanet ./wallplanet
COPY --from=web /src/web/dist /app/web
COPY --from=web /src/web/public/assets /app/web/public/assets
RUN mkdir -p /data && chown wallplanet:wallplanet /data
USER wallplanet
ENV DATA_DIR=/data WEB_DIR=/app/web APP_ADDR=:8080
EXPOSE 8080
ENTRYPOINT ["/app/server/wallplanet"]
