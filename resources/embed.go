/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-11-03 22:13:08
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-11-03 22:35:31
 * @FilePath: \sops-fulla\resources\embed.go
 * @Description: 内嵌配置管理页面的静态资源：编译进二进制，不依赖工作目录
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */

// Package resources 内嵌配置管理页面的静态资源
// 资源编译进二进制，页面不加载任何外部文件，也不依赖工作目录
package resources

import "embed"

// Files 是内嵌的页面资源，路径带 web/ 前缀（web/index.html 等）
//
//go:embed web/index.html web/app.js web/style.css
var Files embed.FS
