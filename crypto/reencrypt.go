/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-11-03 20:05:33
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-11-03 21:15:21
 * @FilePath: \sops-fulla\crypto\reencrypt.go
 * @Description: 最小diff重加密：未变化的值与注释逐字节沿用旧密文
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */

package crypto

import (
	stderrors "errors"

	sops "github.com/getsops/sops/v3"
	"github.com/getsops/sops/v3/aes"
	agekey "github.com/getsops/sops/v3/age"

	"github.com/kamalyes/sops-fulla/errors"
	"github.com/kamalyes/sops-fulla/models"
)

// errUnsupported 是内部信号：这份文件不适合走最小 diff 路径，
// 调用方回退到全量加密（后续校验会给出真实报错）
var errUnsupported = stderrors.New("reencrypt: unsupported")

// EncryptSaving 是保存密文时的统一入口：
// 原文件已是能解开的 SOPS 密文时，只重加密发生变化的值 —— 未变化的值、
// 注释逐字节沿用旧密文，数据密钥也不轮换（与 sops CLI edit 的语义一致），
// git diff 里只出现真正改动的行 + mac / lastmodified；
// 其它情况（新文件、原本是明文、解不开）回退全量加密
func EncryptSaving(root string, original, plain []byte, kind, relPath string, identities agekey.ParsedIdentities) ([]byte, error) {
	if len(original) > 0 && models.IsEncryptedContent(original, kind) {
		output, err := reencrypt(original, plain, kind, relPath, identities)
		switch {
		case err == nil:
			return output, nil
		case errors.Is(err, errUnsupported):
			// 解不开或不适合：交给全量加密处理
		default:
			return nil, err
		}
	}
	return Encrypt(root, plain, kind, relPath)
}

// reencrypt 沿用原数据密钥重加密新明文，机制与 sops CLI edit 相同：
// 解密旧密文时，aes.Cipher 会把每个（明文, 路径）的 IV 记进 stash；
// 再用同一个 cipher 加密新明文时，没变的值命中 stash 复用 IV，
// 密文逐字节一致；变了的值才用新 IV注释与正则 / 后缀规则
// 由 sops 自带的遍历逻辑处理，无需单独适配
func reencrypt(original, plain []byte, kind, relPath string, identities agekey.ParsedIdentities) ([]byte, error) {
	store := SopsStore(kind)
	oldTree, err := store.LoadEncryptedFile(original)
	if err != nil {
		return nil, errUnsupported
	}
	dataKey, err := DataKeyOf(&oldTree, identities)
	if err != nil {
		return nil, errUnsupported
	}
	cipher := aes.NewCipher()
	if _, err := oldTree.Decrypt(dataKey, cipher); err != nil {
		return nil, errUnsupported
	}
	branches, err := loadPlainBranches(store, plain)
	if err != nil {
		return nil, err
	}
	// 沿用旧元数据（age 段、后缀 / 正则规则都不动），只更新 MAC / 时间戳 / 版本
	tree := sops.Tree{Branches: branches, FilePath: relPath, Metadata: oldTree.Metadata}
	return emitEncrypted(store, &tree, dataKey, cipher)
}
