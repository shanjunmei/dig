package model

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"go/ast"
)

type UnusedMode int

const (
	UnusedModeError UnusedMode = iota
	UnusedModeIgnore
	UnusedModeDrop
)

func (m UnusedMode) String() string {
	switch m {
	case UnusedModeError:
		return "error"
	case UnusedModeIgnore:
		return "ignore"
	case UnusedModeDrop:
		return "drop"
	default:
		panic("Unknown Unused Mode")
	}
}

// OpKind 定义闭包操作类型
type OpKind string

const (
	OpDirect  OpKind = "direct"  // 直接返回参数：x
	OpAddr    OpKind = "addr"    // 取地址：&x
	OpDeref   OpKind = "deref"   // 解引用：*x
	OpConvert OpKind = "convert" // 类型转换：T(x)
)

type GenTarget struct {
	FuncName string
	Node     *ast.FuncDecl
	File     string
}
type Arg struct {
	Name       string `json:"name"`
	IsConst    bool   `json:"is_const"`
	ConstValue string `json:"const_value"`
	IsContext  bool   `json:"is_context"`
}

// SchemaVersion identifies the on-disk / wire format of the IR. Bump it whenever
// the Node layout or extraction semantics change, so cached/serialized data is
// invalidated instead of being silently misinterpreted.
const SchemaVersion = 1

// Node is the stable, self-contained intermediate representation produced by the
// extractor and consumed by the generator. Every field is a primitive or a slice
// of primitives, so a Node can be (de)serialized (JSON / gob) without any live
// go/ast or go/types state — which is what enables caching and cross-process use.
type Node struct {
	Name      string `json:"name"`
	Func      string `json:"func"`
	FuncPkg   string `json:"func_pkg"`
	RetType   string `json:"ret_type"`
	Args      []Arg  `json:"args"`
	IsInvoke  bool   `json:"is_invoke"`
	IsSupply  bool   `json:"is_supply"`
	Value     string `json:"value"`
	// ValueIsPkgSymbol records whether the Supply value expression names a
	// package-level symbol (var/func/const/type) in its defining package. When a
	// dig.Supply(value) is inlined into the generation target package, only such
	// package-level symbols need to be qualified as `<FuncPkg>.<value>`;
	// free variables (function parameters and local variables of the inlined
	// module) must NOT be qualified — they are captured by the target function's
	// own scope and referenced directly. A false value means the value is a free
	// variable and must be emitted verbatim.
	ValueIsPkgSymbol bool `json:"value_is_pkg_symbol"`
	HasError  bool   `json:"has_error"`
	IsClosure bool   `json:"is_closure"`

	ClosureDef string   `json:"closure_def"`
	UsedPkgs   []string `json:"used_pkgs"`
	PkgPath    string   `json:"pkg_path"`

	GenericArgs string `json:"generic_args"`

	Comment string `json:"comment"`

	Position string `json:"position"`

	ShouldInline       bool   `json:"should_inline"`
	IsIdentityClosure  bool   `json:"is_identity_closure"`
	IdentityOp         OpKind `json:"identity_op"`
	IdentityTargetType string `json:"identity_target_type"`

	// SchemaVer records the IR schema version this node was serialized with.
	// It is set automatically on Marshal and validated on Unmarshal.
	SchemaVer int `json:"schema_ver"`
}

// CachedExtraction is the serializable bundle produced by the extractor and
// consumed (after a cache hit) by the generator. Because every field is a
// primitive, a slice of primitives, or a string→string map, it can be written
// to disk / sent over the wire (JSON or gob) without any live go/ast or
// go/types state. SchemaVer lets us invalidate stale cache files instead of
// silently misinterpreting them when the Node layout changes.
type CachedExtraction struct {
	SchemaVer int `json:"schema_ver"`

	Nodes []Node `json:"nodes"`

	ImportAliasMap map[string]string `json:"import_alias_map"`
	PkgAliasMap    map[string]string `json:"pkg_alias_map"`
	PkgNameMap     map[string]string `json:"pkg_name_map"`
}

// MarshalJSON serializes the extraction and stamps the current schema version
// onto the bundle and every node, so a cached file is always self-describing.
// The caller's slices are not mutated: nodes are copied before SchemaVer is set.
func (c CachedExtraction) MarshalJSON() ([]byte, error) {
	type shadow CachedExtraction
	s := shadow(c)
	s.SchemaVer = SchemaVersion
	if len(c.Nodes) > 0 {
		nodes := make([]Node, len(c.Nodes))
		for i, n := range c.Nodes {
			n.SchemaVer = SchemaVersion
			nodes[i] = n
		}
		s.Nodes = nodes
	}
	return json.Marshal(s)
}

// UnmarshalJSON decodes the extraction and rejects a bundle whose schema
// version does not match the version this build understands.
func (c *CachedExtraction) UnmarshalJSON(data []byte) error {
	type shadow CachedExtraction
	var tmp shadow
	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}
	*c = CachedExtraction(tmp)
	if c.SchemaVer != SchemaVersion {
		return fmt.Errorf("ir: cache schema version mismatch: got %d, want %d (clear the cache directory to regenerate)", c.SchemaVer, SchemaVersion)
	}
	return nil
}

// EncodeGob gob-encodes the extraction, stamping the schema version. It mutates
// the receiver's node SchemaVer fields (idempotent, harmless) since the caller
// is about to discard or persist them.
func (c *CachedExtraction) EncodeGob() ([]byte, error) {
	c.SchemaVer = SchemaVersion
	for i := range c.Nodes {
		c.Nodes[i].SchemaVer = SchemaVersion
	}
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(c); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// DecodeGob gob-decodes the extraction and validates the schema version.
func (c *CachedExtraction) DecodeGob(data []byte) error {
	if err := gob.NewDecoder(bytes.NewReader(data)).Decode(c); err != nil {
		return err
	}
	if c.SchemaVer != SchemaVersion {
		return fmt.Errorf("ir: gob cache schema version mismatch: got %d, want %d (clear the cache directory to regenerate)", c.SchemaVer, SchemaVersion)
	}
	return nil
}

func init() {
	gob.Register(CachedExtraction{})
	gob.Register(Node{})
	gob.Register(Arg{})
	gob.Register([]Node{})
}

// fullFuncName 返回 包别名.函数名
func FullFuncName(pkgAlias, funcName string) string {
	if pkgAlias == "" {
		return funcName
	}
	return pkgAlias + "." + funcName
}

// LongName 返回用于日志的完整路径（包路径.函数名）
func (node Node) LongName() string {
	if node.PkgPath == "" {
		return node.Func
	}
	return node.PkgPath + "." + node.Func
}
