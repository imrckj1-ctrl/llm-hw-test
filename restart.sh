#!/bin/bash
# 平台后台重启:自动定位脚本所在目录,任何安装路径都能用
# V3.0.2:本机已注册 systemd 用户服务时(如 169 正式机),交 systemd 托管重启——
# 切勿 pkill+nohup 双管理:systemd 会每 3 秒拉起冲突实例,每次启动都把执行中任务误标「中断」
cd "$(dirname "$(readlink -f "$0")")"
if systemctl --user cat hwtest.service >/dev/null 2>&1; then
  systemctl --user restart hwtest.service
  sleep 2
  systemctl --user is-active hwtest.service
  exit 0
fi
pkill -x hwtest 2>/dev/null
sleep 1
setsid nohup ./hwtest -addr 0.0.0.0:8080 > /tmp/hwtest.log 2>&1 < /dev/null &
sleep 2
curl -s -o /dev/null -w 'HTTP-%{http_code}\n' http://127.0.0.1:8080/
