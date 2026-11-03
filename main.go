/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-11-03 20:50:37
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-11-03 21:59:05
 * @FilePath: \sops-fulla\main.go
 * @Description: 工具薄入口：版本注入与bootstrap分发
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */

// sops-fulla 是通用的本地配置仓库管理面板：
// 通过 --repo 指向任意存放 YAML / JSON 配置、用 SOPS(age) 加密
// *.enc.* 文件的仓库，提供浏览器编辑页面（保存时自动加解密，
// 不需要安装 sops）与密文批量管理子命令
package main

import (
	"os"

	"github.com/kamalyes/sops-fulla/bootstrap"
)

// Version 由构建时的 -ldflags "-X main.Version=..." 注入
var Version = "dev"

func main() {
	os.Exit(bootstrap.Run(os.Args[1:], Version))
}
