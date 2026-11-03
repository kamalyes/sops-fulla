/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-11-03 13:52:07
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-11-03 15:07:19
 * @FilePath: \sops-fulla\models\document.go
 * @Description: 保序文档值模型：YAML 与 JSON 都归一到 Value 再校验比较
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */

package models

import (
	"bytes"
	"fmt"

	"github.com/kamalyes/sops-fulla/errors"
)

// Kind 是文档节点的类型
type Kind int

const (
	KindNull Kind = iota
	KindBool
	KindNumber
	KindString
	KindSequence
	KindMapping
)

// Value 是保序、查重后的 YAML / JSON 文档节点
// Map 按键的出现顺序保留；重复键在解析阶段就报错
type Value struct {
	Kind Kind
	Tag  string // YAML 标签（!!str / !!int / !!bool …），JSON 也归一到同一套
	Raw  string // 标量文本：bool / number / string 共用
	Seq  []*Value
	Map  []Entry
}

// Entry 是保序映射里的一个键值对
type Entry struct {
	Key   string
	Value *Value
}

// Equal 做规范化比较：只比类型、标签、标量与结构，忽略锚点、注释与缩进
// 映射按键名比较（与 Python dict 语义一致），键的先后不参与
func (v *Value) Equal(other *Value) bool {
	if v == nil || other == nil {
		return v == nil && other == nil
	}
	if v.Kind != other.Kind || v.Tag != other.Tag || v.Raw != other.Raw {
		return false
	}
	switch v.Kind {
	case KindSequence:
		if len(v.Seq) != len(other.Seq) {
			return false
		}
		for i := range v.Seq {
			if !v.Seq[i].Equal(other.Seq[i]) {
				return false
			}
		}
	case KindMapping:
		if len(v.Map) != len(other.Map) {
			return false
		}
		for _, entry := range v.Map {
			counterpart := FindEntry(other, entry.Key)
			if counterpart == nil || !entry.Value.Equal(counterpart.Value) {
				return false
			}
		}
	}
	return true
}

// MarshalJSON 保持映射键顺序输出
func (v *Value) MarshalJSON() ([]byte, error) {
	var buffer bytes.Buffer
	if err := v.writeJSON(&buffer); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

// writeJSON 递归输出 JSON 文本
func (v *Value) writeJSON(buffer *bytes.Buffer) error {
	if v == nil {
		buffer.WriteString("null")
		return nil
	}
	switch v.Kind {
	case KindNull:
		buffer.WriteString("null")
	case KindBool, KindNumber:
		buffer.WriteString(v.Raw)
	case KindString:
		AppendJSONString(buffer, v.Raw)
	case KindSequence:
		buffer.WriteByte('[')
		for i, item := range v.Seq {
			if i > 0 {
				buffer.WriteByte(',')
			}
			if err := item.writeJSON(buffer); err != nil {
				return err
			}
		}
		buffer.WriteByte(']')
	case KindMapping:
		buffer.WriteByte('{')
		for i, entry := range v.Map {
			if i > 0 {
				buffer.WriteByte(',')
			}
			AppendJSONString(buffer, entry.Key)
			buffer.WriteByte(':')
			if err := entry.Value.writeJSON(buffer); err != nil {
				return err
			}
		}
		buffer.WriteByte('}')
	default:
		return errors.New(500, errors.BizErrCodeInvalidDocument, "文档类型无效")
	}
	return nil
}

// AppendJSONString 输出 JSON 字符串字面量
func AppendJSONString(buffer *bytes.Buffer, text string) {
	buffer.WriteByte('"')
	for _, r := range text {
		switch r {
		case '"':
			buffer.WriteString(`\"`)
		case '\\':
			buffer.WriteString(`\\`)
		case '\n':
			buffer.WriteString(`\n`)
		case '\r':
			buffer.WriteString(`\r`)
		case '\t':
			buffer.WriteString(`\t`)
		default:
			if r < 0x20 {
				fmt.Fprintf(buffer, `\u%04x`, r)
			} else {
				buffer.WriteRune(r)
			}
		}
	}
	buffer.WriteByte('"')
}

// FindEntry 在映射里查找指定键
func FindEntry(value *Value, key string) *Entry {
	for i := range value.Map {
		if value.Map[i].Key == key {
			return &value.Map[i]
		}
	}
	return nil
}

// HasTopKey 判断顶层是否存在指定键
func HasTopKey(value *Value, key string) bool {
	if value == nil {
		return false
	}
	for _, entry := range value.Map {
		if entry.Key == key {
			return true
		}
	}
	return false
}

// TopKeyCount 统计顶层键数（不计 sops 元数据键），给 status / keys 用
func TopKeyCount(value *Value) int {
	if value == nil {
		return 0
	}
	count := len(value.Map)
	if HasTopKey(value, "sops") {
		count--
	}
	return count
}
