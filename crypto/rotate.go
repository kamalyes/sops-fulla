/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-11-05 00:07:19
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-11-05 00:18:52
 * @FilePath: \sops-fulla\crypto\rotate.go
 * @Description: 密钥轮换：把数据密钥重新封装到新age公钥，值密文与MAC保持不变
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */

package crypto

import (
	"regexp"

	sops "github.com/getsops/sops/v3"
	"github.com/getsops/sops/v3/aes"
	agekey "github.com/getsops/sops/v3/age"

	"github.com/kamalyes/sops-fulla/errors"
)

// ageRuleRe 匹配 .sops.yaml 里 creation_rules 条目的 age 公钥行，
// 用于轮换后同步更新加密规则（保留注释与 path_regex 原样）
var ageRuleRe = regexp.MustCompile(`(?m)^([ \t]*age:[ \t]*)(age1[0-9a-z]+)[ \t]*$`)

// multiRecipientRe 识别一条规则配多把公钥（逗号分隔）的行：
// 这种规则无法安全地自动替换，需要管理员手工处理
var multiRecipientRe = regexp.MustCompile(`(?m)^[ \t]*age:[ \t]*age1[0-9a-z]+\s*,`)

// RotateRecipient 把一份密文的 age 段重新封装到新公钥：数据密钥不变，
// 值密文与 MAC 逐字节不变，git diff 里只有 sops.age 段
// 旧私钥用来解出数据密钥，新公钥用来重新封装；两者都不落盘
func RotateRecipient(ciphertext []byte, kind, recipient string, identities agekey.ParsedIdentities) ([]byte, error) {
	store := SopsStore(kind)
	tree, err := store.LoadEncryptedFile(ciphertext)
	if err != nil {
		return nil, cannotProcess(errors.BizErrCodeInvalidCiphertext, "这份密文不是有效的 SOPS 文件")
	}
	dataKey, err := DataKeyOf(&tree, identities)
	if err != nil {
		return nil, translateKeyError(err)
	}
	// 轮换前核对完整性：被改动过的密文不该被搬到新钥匙下
	// verifyMAC 会把树解开成明文，之后须重新加载密文树
	if err := verifyMAC(&tree, dataKey, aes.NewCipher()); err != nil {
		return nil, err
	}
	if tree, err = store.LoadEncryptedFile(ciphertext); err != nil {
		return nil, cannotProcess(errors.BizErrCodeInvalidCiphertext, "这份密文不是有效的 SOPS 文件")
	}
	masterKey, err := agekey.MasterKeyFromRecipient(recipient)
	if err != nil {
		return nil, errors.New(422, errors.BizErrCodeInvalidAgeKey, "新公钥无效，应以 age1 开头")
	}
	tree.Metadata.KeyGroups = []sops.KeyGroup{{masterKey}}
	if err := tree.Metadata.UpdateMasterKeys(dataKey); err != nil {
		return nil, cannotProcess(errors.BizErrCodeInvalidAgeKey, "用新公钥封装数据密钥失败")
	}
	output, err := store.EmitEncryptedFile(tree)
	if err != nil {
		return nil, cannotProcess(errors.BizErrCodeInvalidCiphertext, "生成新密文失败")
	}
	return output, nil
}

// UpdateRulesRecipient 把 .sops.yaml 里所有 age 公钥换成新值，
// 注释、path_regex 与整体排版保持原样返回（新内容, 是否有改动）
// 一条规则配多把公钥（逗号分隔）时不支持自动替换，返回错误
func UpdateRulesRecipient(rules []byte, recipient string) ([]byte, bool, error) {
	if multiRecipientRe.Match(rules) {
		return nil, false, errors.New(422, errors.BizErrCodeEncryptionRules, ".sops.yaml 里有规则的 age 配了多把公钥，请手工替换后重试")
	}
	replaced := ageRuleRe.ReplaceAllString(string(rules), "${1}"+recipient)
	return []byte(replaced), replaced != string(rules), nil
}
