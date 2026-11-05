#!/bin/sh
# @Author: kamalyes 501893067@qq.com
# @Date: 2026-11-05 13:30:00
# @LastEditors: kamalyes 501893067@qq.com
# @LastEditTime: 2026-11-05 13:30:00
# @FilePath: \sops-fulla\install.sh
# @Description: 一键安装：从 GitHub Releases 下载对应平台的二进制，
#               校验 SHA-256 后装进 INSTALL_DIR（默认 ~/.local/bin）
#
# Copyright (c) 2026 by kamalyes, All Rights Reserved.
#
# 用法：
#   curl -fsSL https://raw.githubusercontent.com/kamalyes/sops-fulla/master/install.sh | sh
#   sh install.sh                 # 安装最新 Release
#   sh install.sh sops-fulla-ab12cd3   # 安装指定 Release 的 tag
# 环境变量：
#   INSTALL_DIR  安装目录，默认 ~/.local/bin（建议已在 PATH 里）
set -eu

REPO="kamalyes/sops-fulla"
VERSION="${1:-latest}"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"

say() { printf '%s\n' "$*"; }
die() { printf '安装失败：%s\n' "$*" >&2; exit 1; }

# 平台识别：只支持 macOS / Linux 的 amd64、arm64
# Windows 用户请从 Releases 页面直接下载 .exe
os=$(uname -s)
case "$os" in
  Darwin) os="darwin" ;;
  Linux) os="linux" ;;
  *) die "不支持的系统 $os（本脚本支持 macOS / Linux；Windows 请直接下载 .exe）" ;;
esac
arch=$(uname -m)
case "$arch" in
  x86_64 | amd64) arch="amd64" ;;
  aarch64 | arm64) arch="arm64" ;;
  *) die "不支持的架构 $arch（本脚本支持 amd64 / arm64）" ;;
esac
asset="sops-fulla-${os}-${arch}"

command -v curl >/dev/null 2>&1 || die "需要 curl，请先安装它"

# latest 走 GitHub 的固定跳转地址，指定版本走对应 tag，
# 两种方式都不需要解析 API 响应
if [ "$VERSION" = "latest" ]; then
  base="https://github.com/${REPO}/releases/latest/download"
else
  base="https://github.com/${REPO}/releases/download/${VERSION}"
fi

tmpdir=$(mktemp -d)
trap 'rm -rf "$tmpdir"' EXIT

say "==> 下载 ${asset}（${VERSION}）"
curl -fSL --retry 3 --progress-bar -o "$tmpdir/$asset" "${base}/${asset}" \
  || die "下载二进制失败，请确认网络与版本号（${VERSION}）"

# SHA-256 校验：有校验工具就强校验，没有则给出明确警告
# checksums.txt 的行格式与 sha256sum 一致：<哈希>  <文件名>
if command -v sha256sum >/dev/null 2>&1; then
  hasher="sha256sum"
elif command -v shasum >/dev/null 2>&1; then
  hasher="shasum -a 256"
else
  hasher=""
fi
if [ -n "$hasher" ]; then
  say "==> 校验 SHA-256"
  curl -fsSL --retry 3 -o "$tmpdir/checksums.txt" "${base}/checksums.txt" \
    || die "下载 checksums.txt 失败"
  expected=$(grep " ${asset}\$" "$tmpdir/checksums.txt" | awk '{print $1}')
  [ -n "$expected" ] || die "checksums.txt 里找不到 ${asset} 的条目"
  actual=$($hasher "$tmpdir/$asset" | awk '{print $1}')
  [ "$expected" = "$actual" ] || die "SHA-256 不匹配（期望 $expected，实际 $actual）"
else
  say "==> 警告：本机没有 sha256sum / shasum，跳过完整性校验" >&2
fi

# 落位并验证可执行
mkdir -p "$INSTALL_DIR"
chmod +x "$tmpdir/$asset"
mv "$tmpdir/$asset" "$INSTALL_DIR/sops-fulla"
say "==> 已安装到 $INSTALL_DIR/sops-fulla"
"$INSTALL_DIR/sops-fulla" --version

case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *) say ""
     say "注意：$INSTALL_DIR 不在 PATH 里，可把它加进 shell 配置："
     say "  export PATH=\"$INSTALL_DIR:\$PATH\"" ;;
esac

say "==> 完成！快速上手："
say "  sops-fulla --repo /path/to/configs        # 打开管理页面"
say "  sops-fulla --repo /path/to/configs init   # 从零初始化新仓库"
