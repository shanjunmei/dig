package extractor

import (
	"fmt"
	"github.com/shanjunmei/dig/internal/model"
	"go/token"
	"go/types"
	"golang.org/x/tools/go/packages"
	"strings"
)

func (e *Extractor) checkProviderContextParams(item extractedItem, pos token.Position) error {
	if item.IsInvoke {
		return nil
	}
	for _, p := range item.Params {
		if p.IsContext {
			return fmt.Errorf("at %s: provider %q declares a context.Context parameter %q, "+
				"but providers are resolved eagerly inside InitApp before the runtime context.Context is available. "+
				"Context injection is only supported inside dig.Invoke (which runs within func(context.Context) error)\n"+
				"  💡 Fix: remove the context.Context parameter from the constructor, then either pass the needed value via "+
				"dig.Supply(...), or move the context-dependent work into a dig.Invoke(func(ctx context.Context) {...})",
				pos, e.describeItemByIt(item), p.Name)
		}
	}
	return nil
}

func (e *Extractor) checkGenerationVisibility(obj types.Object, curPkg *packages.Package) error {
	if obj == nil {
		return nil
	}

	var pkg *types.Package
	var name string

	switch o := obj.(type) {
	case *types.Func:
		pkg = o.Pkg()
		name = o.Name()
	case *types.Var:
		// 局部变量与函数/闭包参数是“非包级”符号：它们只存在于将要内联进
		// 生成目标包（mainPkg）的闭包/函数体中，永远不会构成跨包引用。
		// 若不加区分地按包级 var 处理，会把合法的闭包参数（如
		// dig.Invoke(func(f func() Config){ ... f() ... }) 中的 f）误报为
		// “在定义包中是 private、无法被生成目标包使用”。
		// 只有包级 var 才可能跨包被引用，必须放行非包级 var，保留对包级
		// 未导出 var 的校验。
		if !isPackageLevelVar(o) {
			return nil
		}
		pkg = o.Pkg()
		name = o.Name()
	case *types.Const:
		pkg = o.Pkg()
		name = o.Name()
	case *types.TypeName:
		pkg = o.Pkg()
		name = o.Name()
	default:
		return nil
	}

	if pkg == nil {
		return nil
	}
	if pkg.Path() == e.mainPkgPath {
		return nil
	}
	if isExported(name) {
		return nil
	}

	pos := curPkg.Fset.Position(obj.Pos())
	return fmt.Errorf("at %s: %s %q is private in package %s and cannot be used from package %s (generation target)",
		pos,
		strings.ToLower(strings.TrimPrefix(fmt.Sprintf("%T", obj), "*types.")),
		name, pkg.Path(), e.mainPkgPath)
}

// isPackageLevelVar reports whether v is a package-scoped variable (as opposed to
// a local variable or a function/closure parameter).
//
// Only package-level vars can be referenced across packages, so local vars and
// parameters must never be flagged by checkGenerationVisibility: they live inside
// the closure/function body that digen inlines into the generation target package
// and therefore never constitute a cross-package reference.
//
// Detection: a package-level var's declaring scope (Parent) is exactly the
// package scope (Pkg().Scope()), whereas local vars and parameters live in a
// nested (function/block) scope. Function parameters and local vars may carry a
// non-nil Pkg() (the defining package), so the existing `pkg == nil` guard alone
// is insufficient to skip them.
func isPackageLevelVar(v *types.Var) bool {
	if v.Pkg() == nil {
		return false
	}
	// v.Parent() is set when the var is inserted into a scope. Package-level vars
	// are inserted into the package scope; locals/params into a nested scope.
	return v.Parent() == v.Pkg().Scope()
}

func (e *Extractor) buildProviderNotFoundError(typeString, requiredName string, it extractedItem) error {
	available := e.getAvailableProviders(typeString)

	var hint string
	var fix string

	if len(available) == 0 {
		hint = " (no provider for this type at all)"
		fix = "\n  💡 Fix: add a provider for " + typeString + " via dig.Provide or dig.Supply"
	} else {
		hint = " (available: " + strings.Join(available, ", ") + ")"

		var namedOnly []string
		hasDefault := false
		for _, name := range available {
			if name == "(default)" {
				hasDefault = true
			} else {
				namedOnly = append(namedOnly, name)
			}
		}

		if requiredName == "" {
			if len(namedOnly) == 0 {
				// Only default exists
				fix = "\n  💡 Fix: ensure the default provider is in scope"
			} else if len(namedOnly) == 1 && !hasDefault {
				fix = "\n  💡 Fix: rename parameter to '" + namedOnly[0] + "' to match the only named provider, " +
					"or add a default provider via dig.Provide"
			} else if len(namedOnly) == 1 && hasDefault {
				fix = "\n  💡 Fix: rename parameter to '" + namedOnly[0] + "' to use the named provider, " +
					"or use a default provider (no name match)"
			} else {
				fix = "\n  💡 Fix: rename parameter to one of [" + strings.Join(namedOnly, ", ") + "], " +
					"or add a default provider via dig.Provide"
			}
		} else {
			// User requested a specific named provider
			if hasDefault {
				fix = "\n  💡 Fix: check that the provider with name '" + requiredName + "' exists, " +
					"or remove the name from the parameter to use the default provider"
			} else if len(namedOnly) == 1 {
				fix = "\n  💡 Fix: rename parameter to '" + namedOnly[0] + "' (matches the only named provider), " +
					"or remove the name from the provider's return value to make it default"
			} else {
				fix = "\n  💡 Fix: rename parameter to one of [" + strings.Join(namedOnly, ", ") + "]"
			}
		}
	}

	funcName := model.FullFuncName(it.Pkg.PkgPath, it.FuncName)
	if it.IsClosure {
		funcName = it.FuncName + " (closure)"
	}

	nameSuffix := ""
	if requiredName != "" {
		nameSuffix = fmt.Sprintf(" with name %q", requiredName)
	}

	return fmt.Errorf("no provider for type %s%s required by %s at %s%s%s",
		typeString, nameSuffix, funcName, it.Position, hint, fix)
}
