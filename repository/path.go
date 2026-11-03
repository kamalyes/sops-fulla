/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-11-03 15:37:52
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-11-03 17:01:20
 * @FilePath: \sops-fulla\repository\path.go
 * @Description: 路径解析与安全校验：兼容任意深度放置，任何越界都被拒绝
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */

package repository

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/kamalyes/sops-fulla/errors"
)

// nameRe 限制路径各段允许的字符，天然拒绝 `..` 与分隔符
var nameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)

// validName 校验单段名称：字母或数字开头，只含字母、数字、下划线、点、短横线
func validName(value string) bool {
	return nameRe.MatchString(value) && !strings.HasPrefix(value, ".")
}

// isSymlink 判断路径是否为符号链接
func isSymlink(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode()&os.ModeSymlink != 0
}

// resolvePath 解析真实路径；目标不存在时退化为解析父目录
func resolvePath(path string) (string, error) {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved, nil
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil {
		return "", err
	}
	return filepath.Join(parent, filepath.Base(path)), nil
}

// withinRoot 判断目标是否仍在仓库内
func withinRoot(root, target string) bool {
	relative, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

// Resolve 把（服务, 环境, 文件名）换算成仓库内的绝对路径
// 兼容任意放置：服务与环境可为空，环境允许携带多级路径（如 team/dev）
// 任何越界（`..`、符号链接、不在仓库内的解析结果）都被拒绝
func (r *Repository) Resolve(service, environment, filename string) (string, error) {
	if filename == ".sops.yaml" {
		return "", errors.New(400, errors.BizErrCodeInvalidRequest, "只支持 YAML 或 JSON 配置文件")
	}
	segments := relativeSegments(service, environment, filename)
	if segments == nil {
		return "", errors.New(400, errors.BizErrCodeInvalidName, "无效的服务、环境或文件名")
	}
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".yaml", ".yml", ".json":
	default:
		return "", errors.New(400, errors.BizErrCodeUnsupportedFileType, "只支持 YAML 或 JSON 配置文件")
	}
	// 逐级检查中间目录：必须是真实目录且不是符号链接
	current := r.Root
	for _, segment := range segments[:len(segments)-1] {
		current = filepath.Join(current, segment)
		if isSymlink(current) {
			return "", errors.New(404, errors.BizErrCodePathNotFound, "无法访问该配置路径")
		}
		if info, err := os.Lstat(current); err != nil || !info.IsDir() {
			return "", errors.New(404, errors.BizErrCodePathNotFound, "无法访问该配置路径")
		}
	}
	target := filepath.Join(append([]string{r.Root}, segments...)...)
	if isSymlink(target) {
		return "", errors.New(404, errors.BizErrCodePathNotFound, "无法访问该配置路径")
	}
	resolvedTarget, err := resolvePath(target)
	if err != nil || !withinRoot(r.Root, resolvedTarget) {
		return "", errors.New(404, errors.BizErrCodePathNotFound, "无法访问该配置路径")
	}
	if _, err := os.Lstat(target); os.IsNotExist(err) && !r.IsSecretsFile(filename) {
		return "", errors.New(404, errors.BizErrCodeFileNotFound, "配置文件不存在")
	}
	return target, nil
}

// relativeSegments 拼出相对路径的各段并逐段校验；任何一段非法即返回 nil
func relativeSegments(service, environment, filename string) []string {
	var segments []string
	for _, part := range []string{service, environment, filename} {
		if part == "" {
			continue
		}
		for _, segment := range strings.Split(part, "/") {
			if !validName(segment) {
				return nil
			}
			segments = append(segments, segment)
		}
	}
	if len(segments) == 0 {
		return nil
	}
	return segments
}
