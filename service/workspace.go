/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-11-05 10:20:00
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-11-05 10:20:00
 * @FilePath: \sops-fulla\service\workspace.go
 * @Description: 本机工作区记忆：记住的私钥文件与最近打开的仓库列表
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */

package service

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/kamalyes/sops-fulla/constants"
	"github.com/kamalyes/sops-fulla/errors"
)

// HasSopsRules 判断目录里是否有 .sops.yaml 加密规则文件
func HasSopsRules(root string) bool {
	info, err := os.Stat(filepath.Join(root, ".sops.yaml"))
	return err == nil && !info.IsDir()
}

// appConfigDir 返回本工具在用户配置目录下的专属目录：
// 可用 FULLA_CONFIG_DIR 整体覆盖（测试隔离或自定义位置）；
// 默认 Linux ~/.config/sops-fulla、macOS ~/Library/Application Support/sops-fulla、
// Windows %AppData%\sops-fulla
func appConfigDir() (string, error) {
	if override := os.Getenv(constants.EnvConfigDir); override != "" {
		return override, nil
	}
	if config, err := os.UserConfigDir(); err == nil {
		return filepath.Join(config, constants.AppConfigDirName), nil
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".config", constants.AppConfigDirName), nil
	}
	return "", errors.New(500, errors.BizErrCodeInitFailed, "找不到本机存放工具配置的目录")
}

// rememberedKeyPath 返回记住的 age 私钥文件路径；只在本机存放，不进仓库
func rememberedKeyPath() (string, error) {
	dir, err := appConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, constants.RememberedKeyFileName), nil
}

// workspacesFile 返回最近工作区列表文件路径
func workspacesFile() (string, error) {
	dir, err := appConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, constants.WorkspacesFileName), nil
}

// RecentWorkspaces 读取最近打开的工作区列表；读不到或已失效时返回空列表
// 只保留目录仍存在且含 .sops.yaml 的条目，避免页面上出现打不开的仓库
func RecentWorkspaces() []string {
	path, err := workspacesFile()
	if err != nil {
		return []string{}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return []string{}
	}
	var recent []string
	if json.Unmarshal(data, &recent) != nil {
		return []string{}
	}
	valid := make([]string, 0, len(recent))
	seen := map[string]bool{}
	for _, item := range recent {
		if item == "" || seen[item] || !HasSopsRules(item) {
			continue
		}
		seen[item] = true
		valid = append(valid, item)
	}
	return valid
}

// TouchWorkspace 把工作区提到最近列表最前；持久化失败静默处理，
// 不影响切换本身
func TouchWorkspace(root string) {
	if root == "" {
		return
	}
	path, err := workspacesFile()
	if err != nil {
		return
	}
	updated := append([]string{root}, RecentWorkspaces()...)
	if len(updated) > constants.MaxRecentWorkspaces {
		updated = updated[:constants.MaxRecentWorkspaces]
	}
	data, err := json.Marshal(updated)
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return
	}
	os.WriteFile(path, data, 0o600)
}
