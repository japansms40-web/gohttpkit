// Package versionreg 提供「按版本隔离的协议实现」这一模式的骨架：一个类型安全的版本注册表，
// 加上按 endpoint 组织的 header 白名单 / 参数表访问器。
//
// 解决的问题：当你在复刻某个持续演进的私有 API 时，不同客户端版本的请求头集合、
// 参数名、接口地址都会变。把它们塞进同一份实现里加 if version >= X 判断，最终会烂成
// 一团谁都不敢动的分支；而每个版本各写一份完全独立的实现，则需要一个注册与分发机制
// —— 就是本包。
//
// 用法（仿 database/sql 的驱动自注册）：
//
//	// versions/v1/config.go
//	func init() { versions.Registry.MustRegister(cfg) }
//
//	// 使用方
//	cfg, err := versions.Registry.Get("v1.2.3")
//
// 各版本包用一行 blank import 接入分发；新增版本不需要改任何已有代码。
package versionreg

import (
	"fmt"
	"sort"
	"sync"
)

// ID 版本标识，如 "429.1.0.44.70" / "web-2026-06-22"。
// 定义成独立类型而不是裸 string：版本号会在很多地方被传来传去，
// 类型化之后传错参数是编译错误而不是运行时找不到版本。
type ID string

// String 实现 fmt.Stringer。
func (id ID) String() string { return string(id) }

// Versioned 是可注册配置需要满足的最小契约。
type Versioned interface {
	// VersionID 返回本配置的版本标识（注册表的键）。
	VersionID() ID
	// Validate 在注册时被调用，返回错误即在进程启动期 panic。
	// 把「配置写漏了一个必填字段」暴露在启动那一刻，而不是半夜跑到那条分支时。
	Validate() error
}

// Registry 版本注册表。零值不可用，用 New 构造。
//
// 并发模型：注册通常发生在 init()（单线程），但 Get / List 会在请求热路径上被并发调用，
// 故仍用 RWMutex 保护，允许运行期动态注册。
type Registry[T Versioned] struct {
	mu    sync.RWMutex
	items map[ID]T
	name  string
}

// New 创建一个注册表。name 用于错误信息（如 "android versions"）。
func New[T Versioned](name string) *Registry[T] {
	return &Registry[T]{items: make(map[ID]T), name: name}
}

// MustRegister 注册一个版本配置。校验失败或版本重复直接 panic —— 这是启动期 fail-fast，
// 不是运行期错误处理：一个注册不上的版本配置意味着代码或配置写错了，越早炸越好。
func (r *Registry[T]) MustRegister(cfg T) {
	if err := cfg.Validate(); err != nil {
		panic(fmt.Sprintf("versionreg[%s]: 版本 %q 配置非法: %v", r.name, cfg.VersionID(), err))
	}
	id := cfg.VersionID()
	if id == "" {
		panic(fmt.Sprintf("versionreg[%s]: 版本标识不能为空", r.name))
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.items[id]; dup {
		panic(fmt.Sprintf("versionreg[%s]: 版本 %q 重复注册", r.name, id))
	}
	r.items[id] = cfg
}

// Get 取版本配置。版本为空或未注册时返回错误，【不回退默认版本】——
// 静默回退会让「配置漏填」表现成「行为莫名其妙不对”，排查成本极高。
func (r *Registry[T]) Get(id ID) (T, error) {
	var zero T
	if id == "" {
		return zero, fmt.Errorf("versionreg[%s]: 必须指定版本(已注册: %v)", r.name, r.List())
	}
	r.mu.RLock()
	cfg, ok := r.items[id]
	r.mu.RUnlock()
	if !ok {
		return zero, fmt.Errorf("versionreg[%s]: 不支持的版本 %q(已注册: %v)", r.name, id, r.List())
	}
	return cfg, nil
}

// MustGet 取版本配置，取不到就 panic。仅用于「版本来自常量、取不到即代码错误」的场景。
func (r *Registry[T]) MustGet(id ID) T {
	cfg, err := r.Get(id)
	if err != nil {
		panic(err.Error())
	}
	return cfg
}

// List 列出已注册版本（按字典序，便于稳定输出到错误信息与日志）。
func (r *Registry[T]) List() []ID {
	r.mu.RLock()
	ids := make([]ID, 0, len(r.items))
	for id := range r.items {
		ids = append(ids, id)
	}
	r.mu.RUnlock()
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

// Has 判断版本是否已注册。
func (r *Registry[T]) Has(id ID) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.items[id]
	return ok
}

// Len 返回已注册版本数。
func (r *Registry[T]) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.items)
}
