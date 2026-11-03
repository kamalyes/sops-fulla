/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-11-03 18:52:19
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-11-03 19:50:37
 * @FilePath: \sops-fulla\service\keys.go
 * @Description: age私钥管理：页面优先、环境变量与本机密钥文件兜底
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */

package service

import (
	"os"
	"path/filepath"
	"strings"

	agekey "github.com/getsops/sops/v3/age"

	"github.com/kamalyes/sops-fulla/constants"
	"github.com/kamalyes/sops-fulla/errors"
	"github.com/kamalyes/sops-fulla/models"
)

// environmentKey 按 sops CLI 的约定读环境里的 age 私钥：
// SOPS_AGE_KEY、SOPS_AGE_KEY_FILE，以及标准的本机密钥文件路径
func environmentKey() (secret, source string) {
	if key := os.Getenv(constants.EnvAgeKey); strings.TrimSpace(key) != "" {
		return key, constants.KeySourceEnv
	}
	if file := os.Getenv(constants.EnvAgeKeyFile); file != "" {
		if data, err := os.ReadFile(file); err == nil && strings.TrimSpace(string(data)) != "" {
			return string(data), constants.KeySourceFile
		}
		return "", ""
	}
	for _, candidate := range standardAgeKeyPaths() {
		if data, err := os.ReadFile(candidate); err == nil && strings.TrimSpace(string(data)) != "" {
			return string(data), constants.KeySourceFile
		}
	}
	return "", ""
}

// standardAgeKeyPaths 返回 sops CLI 约定的本机密钥文件候选路径，
// 按各操作系统的用户配置目录解析（Linux ~/.config、macOS ~/Library、
// Windows %AppData%），并保留家目录写法作为兜底
func standardAgeKeyPaths() []string {
	if config, err := os.UserConfigDir(); err == nil {
		return []string{filepath.Join(config, "sops", "age", "keys.txt")}
	}
	if home, err := os.UserHomeDir(); err == nil {
		return []string{
			filepath.Join(home, ".config", "sops", "age", "keys.txt"),
			filepath.Join(home, "Library", "Application Support", "sops", "age", "keys.txt"),
		}
	}
	return nil
}

// StandardAgeKeyPath 返回本机 age 私钥文件的首选路径；init 子命令往这里写，
// 之后 service.New 不用任何配置就能读到没有可用的用户配置目录时报错
func StandardAgeKeyPath() (string, error) {
	paths := standardAgeKeyPaths()
	if len(paths) == 0 {
		return "", errors.New(500, errors.BizErrCodeInitFailed, "找不到本机 age 私钥的存放目录")
	}
	return paths[0], nil
}

// keySourceLocked 返回当前生效的私钥来源（须持有 keyMu）
func (s *Service) keySourceLocked() string {
	if s.pageSecret != "" {
		return constants.KeySourcePage
	}
	return s.envSource
}

// identitiesLocked 返回当前生效的 age 私钥，页面优先、其次环境（须持有 keyMu）
func (s *Service) identitiesLocked() agekey.ParsedIdentities {
	if s.pageSecret != "" {
		return s.parsedPage
	}
	if s.envSecret != "" && s.parsedEnv == nil {
		identities := agekey.ParsedIdentities{}
		if err := identities.Import(s.envSecret); err == nil && len(identities) > 0 {
			s.parsedEnv = identities
		} else {
			// 材料在但解析不了：按「不可用」处理，capabilities 会反映出来
			s.parsedEnv = agekey.ParsedIdentities{}
		}
	}
	return s.parsedEnv
}

// Identities 返回当前生效的 age 私钥
func (s *Service) Identities() agekey.ParsedIdentities {
	s.keyMu.Lock()
	defer s.keyMu.Unlock()
	return s.identitiesLocked()
}

// Capabilities 汇报当前可用的能力
func (s *Service) Capabilities() models.Capabilities {
	s.keyMu.Lock()
	defer s.keyMu.Unlock()
	return s.capabilitiesLocked()
}

// capabilitiesLocked 汇报当前可用的能力（须持有 keyMu）
func (s *Service) capabilitiesLocked() models.Capabilities {
	return models.Capabilities{
		SecretsReady: len(s.identitiesLocked()) > 0,
		Repository:   filepath.Base(s.Root()),
		KeySource:    s.keySourceLocked(),
	}
}

// SetPageKey 应用或清除页面提供的 age 私钥；空串表示清除，回落到环境变量
// 私钥只保存在进程内存里，不会写进任何文件或日志
func (s *Service) SetPageKey(secret string) (models.Capabilities, error) {
	secret = strings.TrimSpace(secret)
	s.keyMu.Lock()
	defer s.keyMu.Unlock()
	if secret != "" {
		identities := agekey.ParsedIdentities{}
		if err := identities.Import(secret); err != nil || len(identities) == 0 {
			return models.Capabilities{}, errors.New(400, errors.BizErrCodeInvalidAgeKey, "这不是有效的 age 私钥，应以 AGE-SECRET-KEY- 开头")
		}
		s.pageSecret = secret
		s.parsedPage = identities
	} else {
		s.pageSecret = ""
		s.parsedPage = nil
	}
	return s.capabilitiesLocked(), nil
}
