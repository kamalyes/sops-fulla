/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-11-03 15:00:37
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-11-03 15:10:05
 * @FilePath: \sops-fulla\models\catalog.go
 * @Description: 仓库目录模型：服务 / 环境 / 文件三级，携带私钥能力信息
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */

package models

// Capabilities 描述页面当前可做什么
type Capabilities struct {
	SecretsReady bool `json:"secrets_ready"`
	// HasWorkspace 表示是否已选择配置仓库；false 时页面显示引导卡片
	HasWorkspace bool   `json:"has_workspace"`
	Repository   string `json:"repository"`
	// KeySource 是当前生效的私钥来源：page / env / remembered / file，空串表示没有
	KeySource string `json:"key_source"`
}

// FileEntry 是目录里的一个配置文件
type FileEntry struct {
	Name   string `json:"name"`
	Kind   string `json:"kind"` // secrets | config
	Exists bool   `json:"exists"`
}

// Environment 是一个环境目录
type Environment struct {
	Name  string      `json:"name"`
	Files []FileEntry `json:"files"`
}

// Service 是一个服务目录
type Service struct {
	Name         string        `json:"name"`
	Environments []Environment `json:"environments"`
}

// Catalog 是页面左侧的服务目录
type Catalog struct {
	Services []Service `json:"services"`
	Capabilities
}
