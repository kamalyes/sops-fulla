/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-11-03 19:32:08
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-11-03 19:57:02
 * @FilePath: \sops-fulla\service\realfile_test.go
 * @Description: 用真实仓库密文做兼容性验证（环境变量触发，默认跳过）
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */

package service_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kamalyes/sops-fulla/crypto"
	"github.com/kamalyes/sops-fulla/models"
	"github.com/kamalyes/sops-fulla/service"
)

// TestRealCiphertextParses 用真实仓库里的密文验证兼容性：
// 设置 FULLA_REAL_CIPHERTEXT 指向密文文件、SOPS_AGE_KEY 提供私钥后启用
func TestRealCiphertextParses(t *testing.T) {
	path := os.Getenv("FULLA_REAL_CIPHERTEXT")
	if path == "" {
		t.Skip("设置 FULLA_REAL_CIPHERTEXT 指向真实密文文件后启用")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	kind := models.FileKind(filepath.Base(path))
	if !models.IsEncryptedContent(data, kind) {
		t.Fatal("目标文件不是 SOPS 密文")
	}
	svc := service.New(filepath.Dir(path))
	plain, err := crypto.Decrypt(data, kind, svc.Identities())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := models.ParseDocument(string(plain), kind); err != nil {
		t.Fatalf("解密后的内容不是有效文档：%v", err)
	}
}
