#!/bin/sh
# 在 Linux 服务器上安装或更新 bili-daily（不用 Docker）：
#   下载最新版本到 /opt/bili-daily，校验 SHA-256，装成 systemd 服务并启动。
# 再运行一次就是更新，账号和设置都保留。
#
#   curl -fsSL https://raw.githubusercontent.com/VOfficial404/bili-daily/main/scripts/install.sh | sudo sh
#
# 可以用环境变量改：
#   BILI_DAILY_DIR   安装目录，默认 /opt/bili-daily
#   BILI_DAILY_ADDR  接口监听地址，默认 127.0.0.1:8686（只有服务器本机能连）
set -eu

REPO=VOfficial404/bili-daily
DIR=${BILI_DAILY_DIR:-/opt/bili-daily}
ADDR=${BILI_DAILY_ADDR:-127.0.0.1:8686}
UNIT=/etc/systemd/system/bili-daily.service

[ "$(id -u)" = 0 ] || { echo "请用 root 运行（前面加 sudo）" >&2; exit 1; }
command -v systemctl >/dev/null || { echo "这台机器没有 systemd，请改用 Docker，或者手动运行二进制" >&2; exit 1; }

case "$(uname -m)" in
x86_64 | amd64) arch=amd64 ;;
aarch64 | arm64) arch=arm64 ;;
*) echo "不支持的架构：$(uname -m)" >&2; exit 1 ;;
esac
file="bili-daily_linux_$arch"
base="https://github.com/$REPO/releases/latest/download"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
echo "下载 $file …"
curl -fsSL -o "$tmp/$file" "$base/$file"
curl -fsSL -o "$tmp/checksums.txt" "$base/checksums.txt"
(cd "$tmp" && grep " $file\$" checksums.txt | sha256sum -c -) || { echo "校验失败，没有安装" >&2; exit 1; }

mkdir -p "$DIR"
chmod 700 "$DIR"
install -m 755 "$tmp/$file" "$DIR/bili-daily.new"
mv "$DIR/bili-daily.new" "$DIR/bili-daily"

if [ ! -f "$UNIT" ]; then
	cat >"$UNIT" <<EOF
[Unit]
Description=bili-daily
After=network-online.target
Wants=network-online.target

[Service]
ExecStart=$DIR/bili-daily serve
Environment=BILI_DAILY_DATA=$DIR/data
Environment=BILI_DAILY_ADDR=$ADDR
Restart=on-failure

[Install]
WantedBy=multi-user.target
EOF
	systemctl daemon-reload
	systemctl enable bili-daily >/dev/null
fi
systemctl restart bili-daily

echo
echo "bili-daily $("$DIR/bili-daily" version) 已经在运行。"
echo "  地址：http://$ADDR"
echo "  令牌：$(BILI_DAILY_DATA="$DIR/data" "$DIR/bili-daily" token)"
echo "在客户端里填上面的地址和令牌。地址只有服务器本机能连，从自己电脑连接时先开 SSH 隧道："
echo "  ssh -N -L 8686:$ADDR root@服务器IP"
echo "添加账号：在客户端里点「添加账号」，或者运行"
echo "  BILI_DAILY_DATA=$DIR/data $DIR/bili-daily login"
echo "查看日志：journalctl -u bili-daily -f"
