/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-11-03 15:51:08
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-11-03 17:06:19
 * @FilePath: \sops-fulla\repository\fileio.go
 * @Description: 文件写入与修订号：原子落盘 + 内容指纹乐观并发
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */

package repository

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"

	"github.com/kamalyes/sops-fulla/errors"
)

// AtomicWrite 先写同目录临时文件再原子改名，避免页面读到写了一半的内容
// 目标已存在时沿用原来的权限位，否则按仅属主可读写（0600）创建
func AtomicWrite(path string, data []byte) error {
	if err := atomicWrite(path, data); err != nil {
		return errors.New(500, errors.BizErrCodeWriteFailed, "无法写入配置文件")
	}
	return nil
}

// atomicWrite 是 AtomicWrite 的裸实现，失败时原文件不动
func atomicWrite(path string, data []byte) error {
	mode := os.FileMode(0o600)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp*")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer os.Remove(name)
	if err := temporary.Chmod(mode); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

// RevisionOf 返回内容的修订号（SHA-256 指纹）
func RevisionOf(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

// SameRevision 的语义与旧版一致：两边都没有文件（revision 为 null）也算一致
func SameRevision(data []byte, exists bool, expected *string) bool {
	if !exists || expected == nil {
		return !exists && expected == nil
	}
	return RevisionOf(data) == *expected
}
