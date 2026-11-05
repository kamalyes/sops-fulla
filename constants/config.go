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

// EnvConfigDir 覆盖本工具配置目录（记住的私钥、最近工作区列表）的环境变量
const EnvConfigDir = "FULLA_CONFIG_DIR"

// 私钥来源标识：page（页面填写）> env（环境变量）> file（本机密钥文件）
// remembered 是本工具记住的私钥文件，优先级在标准 sops 密钥文件之前
const (
	KeySourcePage       = "page"
	KeySourceEnv        = "env"
	KeySourceFile       = "file"
	KeySourceRemembered = "remembered"
)

const (
	// AppConfigDirName 本工具在用户配置目录下的专属目录名：
	// 记住的 age 私钥与最近工作区列表都放在这里
	AppConfigDirName = "sops-fulla"
	// RememberedKeyFileName 记住的 age 私钥文件名
	RememberedKeyFileName = "age.key"
	// WorkspacesFileName 最近工作区列表文件名
	WorkspacesFileName = "workspaces.json"
	// MaxRecentWorkspaces 最近工作区列表的保留上限
	MaxRecentWorkspaces = 8
	// AppearanceFileName 页面个性化设置（主题色/字体/字号）文件名
	AppearanceFileName = "appearance.json"
)

// 个性化设置的可选值白名单：与页面下拉/色板一一对应
const (
	AccentPattern = "^#[0-9a-fA-F]{6}$"
)

// AppearanceSansFonts 界面字体可选值
var AppearanceSansFonts = []string{"default", "rounded", "serif", "kai"}

// AppearanceMonoFonts 编辑器字体可选值
var AppearanceMonoFonts = []string{"default", "consolas", "sarasa", "serif"}

// AppearanceZoomLevels 字号档位可选值
var AppearanceZoomLevels = []string{"compact", "standard", "relaxed"}
