# 评测用镜像：交付 Dockerfile 固定在仓库根目录，保留完整 Go 工具链。
FROM golang:1.22
WORKDIR /app
COPY go.mod ./
WORKDIR /app
RUN go mod download
WORKDIR /app
COPY . .
WORKDIR /app
RUN go build ./...
RUN go build -o /app/.runtime-bin ./cmd/server
CMD ["/app/.runtime-bin"]

# 多架构交叉构建示例（请在仓库根目录执行）：
# docker buildx build --platform linux/arm64,linux/amd64 -f benzhi.Dockerfile -t <image> .
