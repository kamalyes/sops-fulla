/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-11-03 18:58:33
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-11-03 19:55:11
 * @FilePath: \sops-fulla\service\secrets.go
 * @Description: 密钥文件形状校验：扁平的大写环境变量名到文本值
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */

package service

import (
	"regexp"

	"github.com/kamalyes/sops-fulla/errors"
	"github.com/kamalyes/sops-fulla/models"
)

// envKeyRe 密钥名必须是合法的环境变量名：大写字母、数字、下划线，且不能以数字开头
var envKeyRe = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)

// SecretPairs 校验密钥文件形状：扁平的 KEY: value，键名是大写环境变量名
func SecretPairs(value *models.Value) ([]models.Entry, error) {
	if value == nil || value.Kind != models.KindMapping {
		return nil, errors.New(400, errors.BizErrCodeSecretsShape, "密钥必须是名称与值的映射")
	}
	for _, entry := range value.Map {
		if !envKeyRe.MatchString(entry.Key) {
			return nil, errors.New(400, errors.BizErrCodeSecretsShape, "密钥名称只能使用大写字母、数字和下划线，不能以数字开头")
		}
		if entry.Value == nil || entry.Value.Kind != models.KindString {
			return nil, errors.New(400, errors.BizErrCodeSecretsShape, "密钥值必须是文本，不能使用嵌套配置")
		}
	}
	return value.Map, nil
}
