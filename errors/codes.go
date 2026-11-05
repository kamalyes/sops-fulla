/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-11-03 13:15:37
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-11-03 13:35:10
 * @FilePath: \sops-fulla\errors\codes.go
 * @Description: 工具专属业务错误码：页面按 Message 展示，Code 供程序判定
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */

package errors

const (
	// ============================================
	// 请求与路径
	// ============================================

	BizErrCodeInvalidRequest      = "error.invalid_request"
	BizErrCodeInvalidName         = "error.invalid_name"
	BizErrCodeUnsupportedFileType = "error.unsupported_file_type"
	BizErrCodePathNotFound        = "error.path_not_found"
	BizErrCodeFileNotFound        = "error.file_not_found"
	BizErrCodeRevisionConflict    = "error.revision_conflict"
	BizErrCodePayloadTooLarge     = "error.payload_too_large"
	BizErrCodeJSONOnly            = "error.json_only"
	BizErrCodeForbidden           = "error.forbidden"
	BizErrCodeUnauthorized        = "error.unauthorized"
	BizErrCodeNotFound            = "error.not_found"
	BizErrCodeNoWorkspace         = "error.no_workspace"
	BizErrCodeWorkspaceInvalid    = "error.workspace_invalid"
	BizErrCodeRememberFailed      = "error.remember_failed"

	// ============================================
	// 文档形状
	// ============================================

	BizErrCodeInvalidDocument  = "error.invalid_document"
	BizErrCodeDuplicateKey     = "error.duplicate_key"
	BizErrCodeSecretsShape     = "error.secrets_shape"
	BizErrCodePrivateKeyLocked = "error.private_key_blocked"

	// ============================================
	// 加解密
	// ============================================

	BizErrCodeInvalidAgeKey     = "error.invalid_age_key"
	BizErrCodeSecretsNotReady   = "error.secrets_not_ready"
	BizErrCodeNoAgeKey          = "error.no_age_key"
	BizErrCodeCipherMismatch    = "error.cipher_mismatch"
	BizErrCodeInvalidCiphertext = "error.invalid_ciphertext"
	BizErrCodeAlreadyEncrypted  = "error.already_encrypted"
	BizErrCodeEncryptionRules   = "error.encryption_rules"
	BizErrCodeVerifyFailed      = "error.verify_failed"
	BizErrCodeWriteFailed       = "error.write_failed"
	BizErrCodeInitFailed        = "error.init_failed"
)
