#!/bin/zsh

# 遇到任何错误立即停止执行
set -e

# 设置输出颜色
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo "${YELLOW}==========================================${NC}"
echo "${YELLOW}🚀 开始重新构建与部署...${NC}"
echo "${YELLOW}==========================================${NC}"

# 1. 重新构建并后台启动容器
echo "\n📦 正在构建 Docker 镜像并启动容器..."
docker rm -f bifrost
docker-compose -f docker-compose-local.yml up -d --build

# 2. 查看容器最新状态
echo "\n🔍 当前容器状态："
docker-compose ps

# 3. 清理旧的悬空镜像 (<none>:<none>)
echo "\n🧹 正在清理构建残留的旧镜像..."
docker image prune -f

echo "\n${GREEN}==========================================${NC}"
echo "${GREEN}✅ 部署完成！网络与服务已成功更新。${NC}"
echo "${GREEN}==========================================${NC}"