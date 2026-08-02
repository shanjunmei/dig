package extractor

import (
	"go/ast"
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
// 策略：基于当前包的传递依赖闭包收集别名，排除其它 main 包和其它 dig.Build 包
// 这样保证 digen ./... 与 digen ./<pkg> 生成结果一致
func (m *AliasManager) LoadImportAliases() {
	type importInfo struct {
		filePath string
		pkgPath  string
		alias    string
	}
	var infos []importInfo

	// 1. 计算当前包的传递依赖闭包
	closure := m.findTransitiveImportClosure()

	// 2. 找出闭包内需要排除的包（其它 main 包 + 其它 dig.Build 包）
	excludePkgPaths := m.findExcludedPackagesInClosure(closure)

	// 3. 仅遍历闭包内的包，收集 import 别名
	for pkgPath := range closure {
		if excludePkgPaths[pkgPath] {
			continue
		}
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
	for _, info := range infos {
		if _, exists := m.importAliasMap[info.pkgPath]; !exists {
			m.importAliasMap[info.pkgPath] = info.alias
		}
	}

	m.logger.Debugf("[alias] Package %s (closure: %d pkgs, excluded: %d):\n",
		m.mainPkgPath, len(closure), len(excludePkgPaths))
	for pkgPath, alias := range m.importAliasMap {
		m.logger.Debugf("  %s -> %s\n", pkgPath, alias)
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

// findExcludedPackagesInClosure 在闭包内找出需要排除的包
// 排除：其它 main 包 + 其它 dig.Build 包
func (m *AliasManager) findExcludedPackagesInClosure(closure map[string]bool) map[string]bool {
	exclude := make(map[string]bool)
	for pkgPath := range closure {
		if pkgPath == m.mainPkgPath {
			continue
		}
		p := m.pkgMap[pkgPath]
		if p == nil {
			continue
		}
		if p.Name == "main" || containsDigBuild(p) {
			exclude[pkgPath] = true
		}
	}
	return exclude
}

// containsDigBuild 检查包中是否存在 dig.Build 调用
func containsDigBuild(p *packages.Package) bool {
	for _, f := range p.Syntax {
		for _, decl := range f.Decls {
			fnDecl, ok := decl.(*ast.FuncDecl)
			if !ok || fnDecl.Body == nil {
				continue
			}
			if FindDigCallInBlock(fnDecl.Body, p.TypesInfo, "Build") != nil {
				return true
			}
		}
	}
	return false
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
