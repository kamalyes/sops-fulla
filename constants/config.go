/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-11-03 13:08:31
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-11-03 13:31:26
 * @FilePath: \sops-fulla\constants\config.go
 * @Description: 工具运行约定：默认端口、密钥文件名、体积上限与私钥来源标识
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */

package constants

const (
	// DefaultPort 管理页面默认端口
	DefaultPort = 8765
	// DefaultSecretsName 密钥文件的默认约定名
	DefaultSecretsName = "secrets.enc.yaml"
	// MaxBodyBytes 单个配置文件（或请求体）的大小上限
	MaxBodyBytes = 2 * 1024 * 1024
)

// age 私钥的环境变量名，与 sops CLI 保持一致
const (
	EnvAgeKey     = "SOPS_AGE_KEY"
	EnvAgeKeyFile = "SOPS_AGE_KEY_FILE"
)

// 私钥来源标识：page（页面填写）> env（环境变量）> file（本机密钥文件）
const (
	KeySourcePage = "page"
	KeySourceEnv  = "env"
	KeySourceFile = "file"
)
