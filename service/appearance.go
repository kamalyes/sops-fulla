/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-11-05 12:10:00
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-11-05 12:10:00
 * @FilePath: \sops-fulla\service\appearance.go
 * @Description: 页面个性化设置的持久化：主题色/字体/字号存本机配置目录
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */

package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"

	"github.com/kamalyes/sops-fulla/constants"
	"github.com/kamalyes/sops-fulla/errors"
)

// accentRe 校验主题色：#RRGGBB；空串表示跟随默认
var accentRe = regexp.MustCompile(constants.AccentPattern)

// Appearance 是页面个性化设置；跟浏览器 localStorage 里的一份同构，
// 存进本机配置目录后换浏览器也能带过去
type Appearance struct {
	Accent string `json:"accent"`
	Sans   string `json:"sans"`
	Mono   string `json:"mono"`
	Zoom   string `json:"zoom"`
}

// appearanceFile 返回个性化设置文件路径
func appearanceFile() (string, error) {
	dir, err := appConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, constants.AppearanceFileName), nil
}

// LoadAppearance 读取持久化的个性化设置；没有文件或内容非法时返回默认值
func LoadAppearance() Appearance {
	defaults := Appearance{Sans: "default", Mono: "default", Zoom: "standard"}
	path, err := appearanceFile()
	if err != nil {
		return defaults
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return defaults
	}
	var stored Appearance
	if json.Unmarshal(data, &stored) != nil {
		return defaults
	}
	normalizeAppearance(&stored)
	if stored.Sans == "" {
		stored.Sans = defaults.Sans
	}
	if stored.Mono == "" {
		stored.Mono = defaults.Mono
	}
	if stored.Zoom == "" {
		stored.Zoom = defaults.Zoom
	}
	return stored
}

// SaveAppearance 校验并写入个性化设置；0600 权限，只在本机存放
func SaveAppearance(appearance Appearance) error {
	normalizeAppearance(&appearance)
	if appearance.Sans == "" || appearance.Mono == "" || appearance.Zoom == "" {
		return errors.New(400, errors.BizErrCodeInvalidRequest, "字体或字号取值无效")
	}
	path, err := appearanceFile()
	if err != nil {
		return err
	}
	data, err := json.Marshal(appearance)
	if err != nil {
		return errors.New(500, errors.BizErrCodeInitFailed, "个性化设置序列化失败")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return errors.New(500, errors.BizErrCodeRememberFailed, "保存个性化设置失败：无法创建配置目录")
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return errors.New(500, errors.BizErrCodeRememberFailed, "保存个性化设置失败：无法写入本机配置文件")
	}
	return nil
}

// normalizeAppearance 把非法字段归零（空串表示默认），主题色必须 #RRGGBB
func normalizeAppearance(appearance *Appearance) {
	if appearance.Accent != "" && !accentRe.MatchString(appearance.Accent) {
		appearance.Accent = ""
	}
	if !contains(constants.AppearanceSansFonts, appearance.Sans) {
		appearance.Sans = ""
	}
	if !contains(constants.AppearanceMonoFonts, appearance.Mono) {
		appearance.Mono = ""
	}
	if !contains(constants.AppearanceZoomLevels, appearance.Zoom) {
		appearance.Zoom = ""
	}
}

func contains(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}
