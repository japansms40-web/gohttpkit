package errors

// kind.go —— 错误分类接口。写法同 logger.Event：本包只定义接口与默认实现，
// 各系统维护自己的 Kind 常量（建议带系统前缀，如 "insgo.login_required"）。

// Kind 是可跨包实现的错误分类；Name 应返回稳定名称，用于 Error() 文案与日志。
// 判定按值比较（IsKind / KindOf），实现类型须可比较；不可比较的实现一律判为不相等。
type Kind interface {
	Name() string
}

type namedKind string

// Name 返回默认 Kind 的原始名称。
// 输入：值接收者 k。
// 返回：创建时的名称，包括空串。
func (k namedKind) Name() string { return string(k) }

// NewKind 用 name 构造默认 Kind。
// 输入：name 为分类名，不校验、不归一化，空串按字面保留。
// 返回：动态值为 namedKind 的 Kind；同名值相等，可作 map 键。
// 例：var KindLoginRequired = errors.NewKind("insgo.login_required")。
func NewKind(name string) Kind { return namedKind(name) }
