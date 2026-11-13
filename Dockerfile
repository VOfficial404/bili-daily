FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS build
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY api ./api
COPY bili ./bili
COPY engine ./engine
COPY cmd ./cmd
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags "-s -w -X main.version=$VERSION" -o /out/bili-tools ./cmd/bili-tools

# distroless/static 自带 CA 证书，没有 shell
FROM gcr.io/distroless/static-debian12
COPY --from=build /out/bili-tools /usr/local/bin/bili-tools
# 容器里监听所有地址；对外开放到哪里由端口映射决定（compose.yaml 默认只映射到服务器本机）
ENV BILI_TOOLS_DATA=/data \
    BILI_TOOLS_ADDR=0.0.0.0:8686
VOLUME /data
EXPOSE 8686
ENTRYPOINT ["bili-tools"]
CMD ["serve"]
