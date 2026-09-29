package versionreg

import (
	"log/slog"
	"slices"
	"sync"

	kiterrors "github.com/japansms40-web/gohttpkit/errors"
)

// registry.go —— 泛型版本注册表。独立成文件：和 endpoint 白名单、示例配置分开，
// 接入方先认 Register/Get，再决定要不要用 HeaderWhitelists。

// ID 版本标识，如 "429.1.0.44.70" / "web-2026-06-22"。
// 给接入方在各处传来传去；定义成独立类型而不是裸 string，传错参数是编译错误。
// 输入：构造时就是原串，不做 trim。
// 返回：String 原样返回底层 string。
type ID string

// String 实现 fmt.Stringer。
// 输入：接收者是版本标识本身。
// 返回：底层字符串，空 ID 返回 ""。
func (id ID) String() string { return string(id) }

// Versioned 是可注册配置需要满足的最小契约。
// 给各版本包的配置对象实现；注册表只认这两件事。
type Versioned interface {
	// VersionID 返回本配置的版本标识（注册表的键）。
	// 输入：无。返回：非空 ID；空串会在 MustRegister 启动期 panic。
	VersionID() ID
	// Validate 在注册时被调用，返回错误即在进程启动期 panic。
	// 输入：接收者应已填好必填字段。
	// 返回：nil 表示可注册；本包 Config 失败是 *MissingConfigFieldError。
	Validate() error
}

// Registry 版本注册表。零值不可用，用 New 构造。
// 给接入方在 init() 注册、请求路径 Get。
//
// 并发模型：mu 保护 items；name 在 New 之后只读。
// 注册通常发生在 init()（单线程），但 Get / List / Has 会在请求热路径上被并发调用，
// 故仍用 RWMutex 保护，允许运行期动态注册。
type Registry[T Versioned] struct {
	mu    sync.RWMutex
	items map[ID]T
	name  string
}

// New 创建一个注册表。
// 输入：name 仅用于错误信息（如 "android versions"），不会当键。
// 返回：可用的空注册表；零值 Registry 不可用。
// 例：New[*Config]("android") → 空表，Len()==0。
func New[T Versioned](name string) *Registry[T] {
	return &Registry[T]{items: make(map[ID]T), name: name}
}

// MustRegister 注册一个版本配置。
// 给各版本包 init()：校验失败或版本重复直接 panic —— 启动期 fail-fast，不是运行期错误。
// 输入：cfg 必须通过 Validate 且 VersionID 非空；T 为指针时表内保存同一实例，注册后勿改字段。
// 返回：无；失败 panic *errors.Error，分类为 KindRegisterInvalid / KindRegisterEmptyID / KindRegisterDuplicate。
// Validate 错误经 Err 解包，recover 后可 errors.As。
// 例：MustRegister(NewConfig("v1", "https://a.example"))；重复 "v1" panic。
func (r *Registry[T]) MustRegister(cfg T) {
	if err := cfg.Validate(); err != nil {
		panic(&kiterrors.Error{
			Op: opMustRegister, Kind: KindRegisterInvalid,
			Attrs: []slog.Attr{slog.String(registryAttrKey, r.name), slog.String(versionAttrKey, string(cfg.VersionID()))}, Err: err,
		})
	}
	id := cfg.VersionID()
	if id == "" {
		panic(&kiterrors.Error{
			Op: opMustRegister, Kind: KindRegisterEmptyID,
			Attrs: []slog.Attr{slog.String(registryAttrKey, r.name)},
		})
	}
	if dup := !r.insert(id, cfg); dup {
		panic(&kiterrors.Error{
			Op: opMustRegister, Kind: KindRegisterDuplicate,
			Attrs: []slog.Attr{slog.String(registryAttrKey, r.name), slog.String(versionAttrKey, string(id))},
		})
	}
}

// Get 取版本配置。
// 给请求路径：版本来自配置或请求，取不到必须显式失败。【不回退默认版本】。
// 输入：id 为空或未注册都是错误；不会改调用方。
// 返回：命中是表内同一 T（指针类型时共享实例，注册后勿改）；
// 空 id → *EmptyVersionError；未注册 → *UnknownVersionError。
// 失败时 Registered 与判定来自同一把读锁快照，按字典序，是新切片。
// 例：Get("v1") → (cfg, nil)；Get("") → *EmptyVersionError；Get("v9") → *UnknownVersionError。
func (r *Registry[T]) Get(id ID) (T, error) {
	cfg, ids, ok := r.lookup(id)
	if ok {
		return cfg, nil
	}
	sortIDs(ids)
	if id == "" {
		return cfg, &EmptyVersionError{Registry: r.name, Registered: ids}
	}
	return cfg, &UnknownVersionError{Registry: r.name, Requested: id, Registered: ids}
}

// MustGet 取版本配置，取不到就 panic。
// 给「版本来自常量、取不到即代码错误」的场景。
// 输入：id 与 Get 相同。
// 返回：命中的 T；失败 panic(err)，err 仍是 Get 的类型错误，recover 后可 errors.As。
func (r *Registry[T]) MustGet(id ID) T {
	cfg, err := r.Get(id)
	if err != nil {
		panic(err)
	}
	return cfg
}

// List 列出已注册版本（按字典序，便于稳定输出到错误信息与日志）。
// 输入：无。
// 返回：新切片，与表内存储无关；空表返回长度为 0 的切片。
func (r *Registry[T]) List() []ID {
	ids := r.copyIDs()
	sortIDs(ids)
	return ids
}

// insert 在写锁内登记一个版本。
// 输入：id 非空，cfg 已校验。
// 返回：新登记为 true；id 已存在为 false，表不变。
func (r *Registry[T]) insert(id ID, cfg T) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.items[id]; dup {
		return false
	}
	r.items[id] = cfg
	return true
}

// lookup 在同一把读锁内查 id，未命中时顺带拷出已注册 id。
// 输入：id 可为空（空 id 恒未命中）。
// 返回：命中为 (cfg, nil, true)；未命中为 (零值, 未排序的新切片, false)，判定与切片来自同一快照。
func (r *Registry[T]) lookup(id ID) (T, []ID, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if id != "" {
		if cfg, ok := r.items[id]; ok {
			return cfg, nil, true
		}
	}
	var zero T
	return zero, r.copyIDsLocked(), false
}

// copyIDs 在读锁内拷出已注册 id。
// 输入：无。
// 返回：新切片，未排序。
func (r *Registry[T]) copyIDs() []ID {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.copyIDsLocked()
}

// Has 判断版本是否已注册。
// 输入：id 原样比较，不做 trim。
// 返回：已注册 true，否则 false。
func (r *Registry[T]) Has(id ID) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.items[id]
	return ok
}

// Len 返回已注册版本数。
// 输入：无。
// 返回：items 长度，空表为 0。
func (r *Registry[T]) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.items)
}

// copyIDsLocked 复制 items 的 key。
// 输入：调用方必须已持有 r.mu 的读锁或写锁。
// 返回：新切片，未排序；空表是长度为 0 的切片，不共享内部存储。
func (r *Registry[T]) copyIDsLocked() []ID {
	ids := make([]ID, 0, len(r.items))
	for id := range r.items {
		ids = append(ids, id)
	}
	return ids
}

// sortIDs 按字典序排 ID 切片。
// 输入：ids 可被就地排序；nil / 空切片可接受。
// 返回：无，就地改 ids。
func sortIDs(ids []ID) {
	slices.Sort(ids)
}
