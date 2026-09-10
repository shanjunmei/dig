package extractor

import (
	"sort"
	"strings"

	"github.com/shanjunmei/dig/internal/logger"
	"github.com/shanjunmei/dig/pkg/alias"
	"golang.org/x/tools/go/packages"
)

type AliasManager struct {
	mainPkgPath    string
	strategy       alias.AliasStrategy
	pkgMap         map[string]*packages.Package
	pkgAliasMap    map[string]string
	importAliasMap map[string]string
	pkgNameMap     map[string]string

	logger *logger.Logger
}

func NewAliasManager(mainPkgPath string, strategy alias.AliasStrategy, pkgMap map[string]*packages.Package, logger *logger.Logger) *AliasManager {
	return &AliasManager{
		mainPkgPath:    mainPkgPath,
		strategy:       strategy,
		pkgMap:         pkgMap,
		pkgAliasMap:    make(map[string]string),
		importAliasMap: make(map[string]string),
		pkgNameMap:     make(map[string]string),

		logger: logger,
	}
}

// CollectPkgAlias 为包生成/获取别名，并记录包名
func (m *AliasManager) CollectPkgAlias(pkg *packages.Package) string {
	if pkg == nil {
		return ""
	}
	pp := pkg.PkgPath
	m.pkgNameMap[pp] = pkg.Name

	if pp == "" || pp == m.mainPkgPath {
		return ""
	}

	// 优先使用 import 中已有的别名
	if alias, ok := m.importAliasMap[pp]; ok {
		m.pkgAliasMap[pp] = alias
		return alias
	}

	// 若已缓存则直接返回
	if alias, ok := m.pkgAliasMap[pp]; ok {
		return alias
	}

	// 收集已有别名（包括导入别名和已生成的别名）
	existing := make(map[string]bool)
	for _, a := range m.importAliasMap {
		existing[a] = true
	}
	for _, a := range m.pkgAliasMap {
		existing[a] = true
	}

	// 生成新别名
	alias := m.strategy.GenerateAlias(pp, existing)
	m.pkgAliasMap[pp] = alias
	return alias
}

// EnsureAlias 确保某路径有别名，若无则生成
func (m *AliasManager) EnsureAlias(pkgPath string) string {
	if pkgPath == "" || pkgPath == m.mainPkgPath {
		return ""
	}
	if alias, ok := m.pkgAliasMap[pkgPath]; ok {
		return alias
	}
	if pkg, ok := m.pkgMap[pkgPath]; ok {
		return m.CollectPkgAlias(pkg)
	}
	// 不在 pkgMap 中（例如内置类型），生成唯一别名
	existing := make(map[string]bool)
	for _, a := range m.pkgAliasMap {
		existing[a] = true
	}
	alias := m.strategy.GenerateAlias(pkgPath, existing)
	m.pkgAliasMap[pkgPath] = alias
	return alias
}

// LoadImportAliases 从已加载的包中收集 import 别名
// 策略：基于当前包的传递依赖闭包收集别名（BFS 从 main 包出发遍历 Imports）
// 这样保证 digen ./... 与 digen ./<pkg> 生成结果一致
//
// BFS 传递闭包通常已天然不包含「其它 main 包」和「其它含 dig.Build 的库包」
// （因为 Go 禁止 import main 包，库包一般不会被 import 两次）。
// 如出现别名冲突场景可在后续重新引入额外排除逻辑。
func (m *AliasManager) LoadImportAliases() {
	type importInfo struct {
		filePath string
		pkgPath  string
		alias    string
		fromMain bool
	}
	var infos []importInfo

	// The file digen generates always lives in the MAIN package, so its import
	// block must use the names the main package itself uses for each package —
	// never a name chosen by some unrelated dependency. A dependency's explicit
	// alias (e.g. `gotime "time"`) must not leak into the generated file, or the
	// verbatim package selectors copied from the main package's source (which use
	// the main package's names) would desync from the aliased import and fail to
	// type-check (this was the root cause of the digen-generated `time`/`gotime`
	// "undefined: time / imported as gotime and not used" bug).
	//
	// Therefore the main package's own import is authoritative for a given path:
	// if the main package imports a package at all (with or without an explicit
	// alias), any dependency alias for that path is ignored. Only when the main
	// package does NOT import the path do we fall back to a dependency's explicit
	// alias (still needed for the rare cross-package closure that pulls in a
	// package the main package itself never names).
	mainImports := make(map[string]bool)
	if mainPkg := m.pkgMap[m.mainPkgPath]; mainPkg != nil {
		for _, f := range mainPkg.Syntax {
			for _, imp := range f.Imports {
				path := strings.Trim(imp.Path.Value, `"`)
				if path == "" {
					continue
				}
				mainImports[path] = true
			}
		}
	}

	// 1. 计算当前包的传递依赖闭包
	closure := m.findTransitiveImportClosure()

	// 2. 仅遍历闭包内的包，收集 import 别名
	for pkgPath := range closure {
		p := m.pkgMap[pkgPath]
		if p == nil {
			continue
		}
		for _, f := range p.Syntax {
			filePos := p.Fset.Position(f.Pos())
			filePath := filePos.Filename
			for _, imp := range f.Imports {
				path := strings.Trim(imp.Path.Value, `"`)
				if imp.Name != nil {
					alias := imp.Name.Name
					if alias != "." && alias != "_" {
						infos = append(infos, importInfo{
							filePath: filePath,
							pkgPath:  path,
							alias:    alias,
							fromMain: pkgPath == m.mainPkgPath,
						})
					}
				}
			}
		}
	}

	// 排序以保证确定性（按文件路径和包路径）
	sort.Slice(infos, func(i, j int) bool {
		if infos[i].filePath != infos[j].filePath {
			return infos[i].filePath < infos[j].filePath
		}
		return infos[i].pkgPath < infos[j].pkgPath
	})
	// 两趟落库，区分「主包自己的别名」与「依赖的别名」：
	//
	// 第一趟只处理主包自己的显式别名，它们是权威值（生成文件就在主包里，
	// 必须使用主包源码里的本地名，例如 `import ctx "context"` 必须是 ctx）。
	// 第二趟才处理依赖的别名，且仅当主包根本没有导入该路径时才可以兜底；
	// 主包导入过的路径一律不让依赖别名污染（否则逐字拷贝的包选择器会与
	// 导入块脱钩，即 time/gotime 那个 bug）。
	//
	// 注意：不能用 "主包导入过就 continue" 一把筛掉 —— 那样会把主包自己的
	// 显式别名也一起丢掉，生成 `import "context"` 而闭包体写 `ctx.Context`，
	// 反而制造脱钩（example/context_alias 与 shadow_err 的 golden 回归即由此而来）。
	for _, info := range infos {
		if !info.fromMain {
			continue
		}
		m.importAliasMap[info.pkgPath] = info.alias
	}
	for _, info := range infos {
		if info.fromMain || mainImports[info.pkgPath] {
			continue
		}
		if _, exists := m.importAliasMap[info.pkgPath]; !exists {
			m.importAliasMap[info.pkgPath] = info.alias
		}
	}

	m.logger.Debugf("[alias] Package %s (closure: %d pkgs,):",
		m.mainPkgPath, len(closure))
	for pkgPath, alias := range m.importAliasMap {
		m.logger.Debugf("  %s -> %s", pkgPath, alias)
	}

}

// findTransitiveImportClosure 计算从当前包出发的传递依赖闭包
// 通过 packages.Package.Imports 进行 BFS，收集所有可达的包路径
func (m *AliasManager) findTransitiveImportClosure() map[string]bool {
	closure := make(map[string]bool)
	if m.mainPkgPath == "" {
		return closure
	}
	queue := []string{m.mainPkgPath}
	closure[m.mainPkgPath] = true

	for len(queue) > 0 {
		pkgPath := queue[0]
		queue = queue[1:]

		p := m.pkgMap[pkgPath]
		if p == nil {
			continue
		}
		for _, impPkg := range p.Imports {
			if impPkg.PkgPath == "" {
				continue
			}
			if !closure[impPkg.PkgPath] {
				closure[impPkg.PkgPath] = true
				queue = append(queue, impPkg.PkgPath)
			}
		}
	}
	return closure
}

// 查询方法
func (m *AliasManager) GetAlias(pkgPath string) string {
	if pkgPath == "" || pkgPath == m.mainPkgPath {
		return ""
	}
	return m.pkgAliasMap[pkgPath]
}

func (m *AliasManager) GetPkgAliasMap() map[string]string {
	return m.pkgAliasMap
}

func (m *AliasManager) GetImportAliasMap() map[string]string {
	return m.importAliasMap
}

func (m *AliasManager) GetPkgNameMap() map[string]string {
	return m.pkgNameMap
}

func (m *AliasManager) GetMainPkgPath() string {
	return m.mainPkgPath
}

// ForceAlias forces the import for pkgPath to use the given local name. This is
// required when a package is referenced verbatim inside a closure body that digen
// inlines into the main package — for example `time.Duration` in an inlined
// external function body. The body uses the package name exactly as it appears in
// the *source* package (e.g. `time`, or `gotime` if that package aliased it), and
// the generated file's import block MUST use that same local name, otherwise the
// verbatim selector desyncs from the (possibly aliased) import and the generated
// file fails to type-check ("undefined: time" / "imported as gotime and not used").
//
// Because the body is copied verbatim and is NOT rewritten to an alias, this
// method overrides any alias previously collected by LoadImportAliases (including
// a dependency's explicit alias that would otherwise leak in). localName is the
// name used in the body; realName is the package's declared name and is used to
// decide whether the import is emitted unaliased (alias == realName).
func (m *AliasManager) ForceAlias(pkgPath, localName, realName string) {
	if pkgPath == "" || pkgPath == m.mainPkgPath {
		return
	}
	m.importAliasMap[pkgPath] = localName
	m.pkgAliasMap[pkgPath] = localName
	if realName != "" {
		m.pkgNameMap[pkgPath] = realName
	}
}
