#!/bin/bash
# 构建 pi 质量测试沙箱镜像 hwtest-pi:local
# 用法:bash deploy/pi/build_image.sh
set -e
cd "$(dirname "$0")"
command -v docker >/dev/null || { echo "未找到 docker,请先安装 Docker"; exit 1; }
docker build -t hwtest-pi:local -f Dockerfile .
echo "OK: 镜像 hwtest-pi:local 构建完成"
