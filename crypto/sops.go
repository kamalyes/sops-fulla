/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-11-03 17:15:31
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-11-03 18:29:07
 * @FilePath: \sops-fulla\crypto\sops.go
 * @Description: SOPS 原生加解密适配：与 sops CLI 生成的密文完全同构
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */

// Package crypto 封装 SOPS 的原生加解密：输出与 sops CLI 生成的文件同构，
// 页面保存时只需公钥，解密与核对需要 age 私钥
package crypto

import (
	stderrors "errors"
	"time"

	sops "github.com/getsops/sops/v3"
	"github.com/getsops/sops/v3/aes"
	agekey "github.com/getsops/sops/v3/age"
	sopsconfig "github.com/getsops/sops/v3/config"
	sopsjson "github.com/getsops/sops/v3/stores/json"
	sopsyaml "github.com/getsops/sops/v3/stores/yaml"
	"github.com/getsops/sops/v3/version"

	"github.com/kamalyes/sops-fulla/errors"
)

// SopsStore 按文件格式返回对应的 SOPS store
func SopsStore(kind string) sops.Store {
	if kind == "json" {
		return sopsjson.NewStore(&sopsconfig.JSONStoreConfig{})
	}
	return sopsyaml.NewStore(&sopsconfig.YAMLStoreConfig{})
}

// cannotProcess 统一构造 422：处理失败，但原文件保持未修改
func cannotProcess(code, reason string) *errors.Error {
	return errors.New(422, code, "无法处理此配置，原文件未修改："+reason)
}

// 解数据密钥时的内部错误，Decrypt 把它们翻译成对应状态码
var (
	errNoIdentity = stderrors.New("没有可用的 age 私钥")
	errNoAgeKey   = stderrors.New("密文里没有 age 密钥")
)

// DataKeyOf 把当前身份注入每个 age master key 并逐个尝试解出数据密钥
// 不走 tree.Metadata.GetDataKey()：那条路要经过 keyservice 的序列化往返，
// 页面注入的身份会丢，只剩环境变量一条来源
func DataKeyOf(tree *sops.Tree, identities agekey.ParsedIdentities) ([]byte, error) {
	if len(identities) == 0 {
		return nil, errNoIdentity
	}
	applied := false
	var lastErr error
	for _, group := range tree.Metadata.KeyGroups {
		for _, key := range group {
			ageMaster, ok := key.(*agekey.MasterKey)
			if !ok {
				continue
			}
			identities.ApplyToMasterKey(ageMaster)
			applied = true
			part, err := ageMaster.Decrypt()
			if err == nil {
				return part, nil
			}
			lastErr = err
		}
	}
	if !applied {
		return nil, errNoAgeKey
	}
	return nil, lastErr
}

// Decrypt 用当前可用的 age 私钥原生解密，并核对 MAC
// 每次调用使用独立的值加密器，避免并发请求共享内部状态
func Decrypt(ciphertext []byte, kind string, identities agekey.ParsedIdentities) ([]byte, error) {
	cipher := aes.NewCipher()
	store := SopsStore(kind)
	tree, err := store.LoadEncryptedFile(ciphertext)
	if err != nil {
		return nil, cannotProcess(errors.BizErrCodeInvalidCiphertext, "这份密文不是有效的 SOPS 文件")
	}
	dataKey, err := DataKeyOf(&tree, identities)
	if err != nil {
		return nil, translateKeyError(err)
	}
	// verifyMAC 会顺带把树解成明文，正是 EmitPlainFile 需要的形态
	if err := verifyMAC(&tree, dataKey, cipher); err != nil {
		return nil, err
	}
	plain, err := store.EmitPlainFile(tree.Branches)
	if err != nil {
		return nil, cannotProcess(errors.BizErrCodeInvalidCiphertext, "解密失败")
	}
	return plain, nil
}

// verifyMAC 用数据密钥核对文件 MAC；通过后 tree 的值已被解开成明文
// 调用方若还要密文形态，须重新 LoadEncryptedFile
func verifyMAC(tree *sops.Tree, dataKey []byte, cipher aes.Cipher) error {
	computedMAC, err := tree.Decrypt(dataKey, cipher)
	if err != nil {
		return cannotProcess(errors.BizErrCodeInvalidCiphertext, "解密失败")
	}
	fileMAC, err := cipher.Decrypt(tree.Metadata.MessageAuthenticationCode, dataKey, tree.Metadata.LastModified.Format(time.RFC3339))
	if err != nil || fileMAC != computedMAC {
		return errors.New(422, errors.BizErrCodeCipherMismatch, "内容校验不一致，原文件未修改：这份密文可能被改动过")
	}
	return nil
}

// translateKeyError 把解数据密钥的内部错误翻译成面向页面的报错
func translateKeyError(err error) error {
	switch {
	case errors.Is(err, errNoIdentity):
		return errors.New(503, errors.BizErrCodeSecretsNotReady, "密钥编辑尚未就绪，请在页面填写 age 私钥，或由管理员配置 SOPS_AGE_KEY / SOPS_AGE_KEY_FILE")
	case errors.Is(err, errNoAgeKey):
		return errors.New(422, errors.BizErrCodeNoAgeKey, "这份密文没有 age 密钥，无法在页面打开")
	default:
		return errors.Newf(422, errors.BizErrCodeCipherMismatch, "无法处理此配置，原文件未修改：私钥与这份密文不匹配（%v）", err)
	}
}

// Encrypt 用 .sops.yaml 里的 age 公钥原生加密，输出与 sops CLI 生成的文件同构
// 只需要公钥；私钥从不参与加密root 是配置仓库根目录
func Encrypt(root string, plain []byte, kind, relPath string) ([]byte, error) {
	recipient, err := ruleRecipient(root, relPath)
	if err != nil {
		return nil, err
	}
	store := SopsStore(kind)
	branches, err := loadPlainBranches(store, plain)
	if err != nil {
		return nil, err
	}
	masterKey, err := agekey.MasterKeyFromRecipient(recipient)
	if err != nil {
		return nil, cannotProcess(errors.BizErrCodeEncryptionRules, ".sops.yaml 里的 age 公钥无效")
	}
	tree := sops.Tree{
		Branches: branches,
		FilePath: relPath,
		Metadata: sops.Metadata{
			KeyGroups: []sops.KeyGroup{{masterKey}},
			// 与 sops CLI 默认行为一致（现有密文文件里就是这么写的）
			UnencryptedSuffix: sops.DefaultUnencryptedSuffix,
			Version:           version.Version,
		},
	}
	// 生成数据密钥并用公钥包进 master key，全部在内存里完成
	dataKey, errs := tree.GenerateDataKey()
	if len(errs) > 0 {
		return nil, cannotProcess(errors.BizErrCodeEncryptionRules, "请管理员检查仓库加密规则")
	}
	return emitEncrypted(store, &tree, dataKey, aes.NewCipher())
}

// loadPlainBranches 解析明文并做基本形状校验：非空、顶层映射、不是密文
func loadPlainBranches(store sops.Store, plain []byte) (sops.TreeBranches, error) {
	branches, err := store.LoadPlainFile(plain)
	if err != nil {
		return nil, errors.New(400, errors.BizErrCodeInvalidDocument, "配置格式有误，原文件未修改")
	}
	if len(branches) == 0 {
		return nil, errors.New(400, errors.BizErrCodeInvalidDocument, "配置必须是键值映射，不能是空文档或列表")
	}
	if store.HasSopsTopLevelKey(branches[0]) {
		return nil, errors.New(422, errors.BizErrCodeAlreadyEncrypted, "这份文件已经是密文，无需重复加密")
	}
	return branches, nil
}

// emitEncrypted 加密整棵树并补齐元数据（MAC / 时间戳 / 版本），输出密文文件
func emitEncrypted(store sops.Store, tree *sops.Tree, dataKey []byte, cipher aes.Cipher) ([]byte, error) {
	mac, err := tree.Encrypt(dataKey, cipher)
	if err != nil {
		return nil, cannotProcess(errors.BizErrCodeInvalidCiphertext, "加密失败")
	}
	tree.Metadata.LastModified = time.Now().UTC()
	tree.Metadata.Version = version.Version
	// MAC 本身也用数据密钥加密后落盘，附加数据是 LastModified（RFC3339）
	tree.Metadata.MessageAuthenticationCode, err = cipher.Encrypt(mac, dataKey, tree.Metadata.LastModified.Format(time.RFC3339))
	if err != nil {
		return nil, cannotProcess(errors.BizErrCodeInvalidCiphertext, "加密失败")
	}
	output, err := store.EmitEncryptedFile(*tree)
	if err != nil {
		return nil, cannotProcess(errors.BizErrCodeInvalidCiphertext, "加密失败")
	}
	return output, nil
}
