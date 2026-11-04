/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-11-05 00:10:52
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-11-05 00:17:38
 * @FilePath: \sops-fulla\models\api.go
 * @Description: 页面与工具的读写契约：文件数据、保存请求与保存结果
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */

package models

// FileData 是 /api/file 的返回
type FileData struct {
	Revision *string `json:"revision"` // 文件内容的 SHA-256；文件不存在时为 null
	Filename string  `json:"filename"`
	Kind     string  `json:"kind"`              // secrets | config
	Values   *Value  `json:"values,omitempty"`  // 密钥文件：扁平键值
	Content  *string `json:"content,omitempty"` // 普通配置：完整文本
}

// SaveRequest 是 /api/save 的请求
type SaveRequest struct {
	Service     string
	Environment string
	Filename    string
	Revision    *string
	Values      *Value  // 密钥文件：扁平键值
	Content     *string // 普通配置：完整文本
}

// SaveResult 是 /api/save 的返回
type SaveResult struct {
	Revision string `json:"revision"`
	Message  string `json:"message"`
}

// RotateRequest 是 /api/rotate 的请求：新 age 公钥（age1 开头）
type RotateRequest struct {
	Recipient string `json:"recipient"`
}

// RotateResult 是 /api/rotate 的返回：轮换过的文件（仓库内相对路径）
// 与 .sops.yaml 是否一并更新
type RotateResult struct {
	Files        []string `json:"files"`
	RulesUpdated bool     `json:"rules_updated"`
}

// KeyPair 是 /api/keypair 的返回：页面上生成的新 age 密钥对
// 私钥只在响应里出现一次，由使用者自行保存，服务端不留副本
type KeyPair struct {
	Recipient string `json:"recipient"` // 公钥（age1…），进 .sops.yaml
	Identity  string `json:"identity"`  // 私钥（AGE-SECRET-KEY-…），请妥善备份
}
