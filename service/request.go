/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-11-03 19:07:21
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-11-03 19:51:08
 * @FilePath: \sops-fulla\service\request.go
 * @Description: 保存请求体解析：拒绝重复键，保留密钥顺序
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */

package service

import (
	"github.com/kamalyes/sops-fulla/errors"
	"github.com/kamalyes/sops-fulla/models"
)

// ParseSaveRequest 解析保存请求体：拒绝重复键，保留密钥顺序
// 字段形状的问题留给 Save 按文件类型判定，与旧版的报错顺序一致
func ParseSaveRequest(body []byte) (*models.SaveRequest, error) {
	value, err := models.ParseJSON(string(body))
	if err != nil {
		// 只有「重复键」原样透传，其余一律按请求格式无效处理
		if errors.IsCode(err, errors.BizErrCodeDuplicateKey) {
			return nil, err
		}
		return nil, errors.New(400, errors.BizErrCodeInvalidRequest, "请求格式无效")
	}
	if value == nil || value.Kind != models.KindMapping {
		return nil, errors.New(400, errors.BizErrCodeInvalidRequest, "请求格式无效")
	}
	return buildSaveRequest(value)
}

// buildSaveRequest 从解析好的 JSON 对象里取出关心的字段
func buildSaveRequest(value *models.Value) (*models.SaveRequest, error) {
	request := &models.SaveRequest{}
	for _, entry := range value.Map {
		text, isText := stringValue(entry.Value)
		switch entry.Key {
		case "service", "environment", "filename":
			if isText {
				setTarget(request, entry.Key, text)
			}
		case "revision":
			if entry.Value != nil && entry.Value.Kind == models.KindString {
				request.Revision = &text
			} else if entry.Value == nil || entry.Value.Kind == models.KindNull {
				request.Revision = nil
			} else {
				return nil, errors.New(400, errors.BizErrCodeInvalidRequest, "缺少有效的文件版本，请重新加载")
			}
		case "values":
			request.Values = entry.Value
		case "content":
			if isText {
				request.Content = &text
			}
		}
	}
	return request, nil
}

// setTarget 写入定位文件的三个字段之一
func setTarget(request *models.SaveRequest, key, value string) {
	switch key {
	case "service":
		request.Service = value
	case "environment":
		request.Environment = value
	case "filename":
		request.Filename = value
	}
}

// stringValue 取出字符串标量；非字符串返回 false
func stringValue(value *models.Value) (string, bool) {
	if value != nil && value.Kind == models.KindString {
		return value.Raw, true
	}
	return "", false
}
