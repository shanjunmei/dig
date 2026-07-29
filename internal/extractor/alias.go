package extractor

import (
	"sort"
	"strings"

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
}

func NewAliasManager(mainPkgPath string, strategy alias.AliasStrategy, pkgMap map[string]*packages.Package) *AliasManager {
	return &AliasManager{
		mainPkgPath:    mainPkgPath,
		strategy:       strategy,
		pkgMap:         pkgMap,
		pkgAliasMap:    make(map[string]string),
		importAliasMap: make(map[string]string),
		pkgNameMap:     make(map[string]string),
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

// LoadImportAliases 从所有已加载包的 import 语句中收集别名
func (m *AliasManager) LoadImportAliases() {
	type importInfo struct {
		filePath string
		pkgPath  string
		alias    string
	}
	var infos []importInfo
	for _, p := range m.pkgMap {
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
