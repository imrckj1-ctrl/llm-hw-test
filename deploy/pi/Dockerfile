# pi 质量测试沙箱镜像:node(pi 执行器)+ python3(文档生成依赖)+ 中文字体
# 构建:bash deploy/pi/build_image.sh
FROM node:22-bookworm-slim

# 国内镜像加速(海外网络可删除这两行 sed / pip -i 参数)
RUN sed -i 's|deb.debian.org|mirrors.aliyun.com|g; s|security.debian.org|mirrors.aliyun.com/debian-security|g' /etc/apt/sources.list.d/debian.sources 2>/dev/null || true

RUN apt-get update \
 && apt-get install -y --no-install-recommends python3 python3-pip fonts-noto-cjk \
 && rm -rf /var/lib/apt/lists/*

# 文档生成依赖:PDF / Excel / PPT / Word / 图表
RUN pip3 install --no-cache-dir --break-system-packages \
    -i https://mirrors.aliyun.com/pypi/simple/ \
    reportlab openpyxl python-pptx python-docx matplotlib

WORKDIR /work

# 运行方式(平台自动发起):
# docker run --rm --network host -v <scripts/pi>:/pi:ro -v <任务工作目录>:/work \
#   hwtest-pi:local node /pi/runner.mjs /work/pi-payload.json
