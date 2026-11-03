/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-11-03 13:05:18
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-11-03 13:29:52
 * @FilePath: \sops-fulla\errors\errors.go
 * @Description: 带HTTP状态码与错误码的业务错误：页面把它原样转给浏览器
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */

package errors

import (
	stderrors "errors"
	"fmt"
)

// Error 携带 HTTP 状态码与业务错误码；Message 面向页面展示
type Error struct {
	Status  int
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Message }

// New 构造带状态码与错误码的业务错误
func New(status int, code, message string) *Error {
	return &Error{Status: status, Code: code, Message: message}
}

// Newf 构造带格式化消息的业务错误
func Newf(status int, code, format string, args ...any) *Error {
	return &Error{Status: status, Code: code, Message: fmt.Sprintf(format, args...)}
}

// As 取出错误里的业务错误；支持被 %w 包装的情况
func As(err error) (*Error, bool) {
	var business *Error
	if stderrors.As(err, &business) {
		return business, true
	}
	return nil, false
}

// Is 判断错误链里是否含有指定错误，语义同标准库 errors.Is
func Is(err, target error) bool { return stderrors.Is(err, target) }

// IsCode 判断错误是否为指定错误码的业务错误
func IsCode(err error, code string) bool {
	business, ok := As(err)
	return ok && business.Code == code
}
