/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-11-03 17:38:52
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-11-03 18:30:21
 * @FilePath: \sops-fulla\crypto\rules.go
 * @Description: .sops.yaml 加密规则读取：按路径匹配 age 公钥
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */

package crypto

import (
	"os"
	"path/filepath"
	"regexp"

	"gopkg.in/yaml.v3"

	"github.com/kamalyes/sops-fulla/errors"
)

// creationRulesFile 是 .sops.yaml 里关心的字段
type creationRulesFile struct {
	CreationRules []struct {
		PathRegex string `yaml:"path_regex"`
		Age       string `yaml:"age"`
	} `yaml:"creation_rules"`
}

// ruleRecipient 从 .sops.yaml 里找到匹配该文件路径的 age 公钥
// 与 sops CLI 一致：按规则顺序取第一个匹配且带 age 公钥的条目
func ruleRecipient(root, relPath string) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, ".sops.yaml"))
	if err != nil {
		return "", rulesError("读不到 .sops.yaml 加密规则")
	}
	var rules creationRulesFile
	if err := yaml.Unmarshal(data, &rules); err != nil {
		return "", rulesError(".sops.yaml 不是有效的规则文件")
	}
	for _, rule := range rules.CreationRules {
		if rule.Age == "" || rule.PathRegex == "" {
			continue
		}
		matched, err := regexp.MatchString(rule.PathRegex, relPath)
		if err != nil {
			return "", rulesError(".sops.yaml 的 path_regex 无效")
		}
		if matched {
			return rule.Age, nil
		}
	}
	return "", errors.Newf(422, errors.BizErrCodeEncryptionRules,
		"无法处理此配置，原文件未修改：.sops.yaml 里没有匹配 %s 的加密规则", relPath)
}

// rulesError 统一构造 .sops.yaml 相关的 422 错误
func rulesError(reason string) *errors.Error {
	return cannotProcess(errors.BizErrCodeEncryptionRules, reason)
}
