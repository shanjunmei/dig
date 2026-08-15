package extractor

import (
	"fmt"
	"github.com/shanjunmei/dig/internal/model"
	"github.com/shanjunmei/dig/pkg/functional"
	"go/printer"
	"strings"
)

func (e *Extractor) buildDependencyGraph(items []extractedItem) ([][]int, []int, error) {
	n := len(items)
	adj := make([][]int, n)
	indeg := make([]int, n)

	for i, it := range items {
		if it.IsSupply {
			continue
		}
		for _, arg := range it.Params {
			if arg.IsContext {
				continue
			}
			if it.IsClosure && arg.IsConst {
				continue
			}

			providerIdx, err := e.resolveProvider(arg, it)
			if err != nil {
				return nil, nil, err
			}

			adj[providerIdx] = append(adj[providerIdx], i)
			indeg[i]++
		}
	}
	return adj, indeg, nil
}

func (e *Extractor) findCycle(adj [][]int) []int {
	n := len(adj)
	state := make([]int, n) // 0=未访问, 1=访问中, 2=已处理
	parent := make([]int, n)
	for i := range n {
		if state[i] == 0 {
			stack := []int{i}
			state[i] = 1
			parent[i] = -1
			for len(stack) > 0 {
				u := stack[len(stack)-1]
				found := false
				for _, v := range adj[u] {
					if state[v] == 0 {
						state[v] = 1
						parent[v] = u
						stack = append(stack, v)
						found = true
						break
					} else if state[v] == 1 {
						cycle := []int{v}
						for cur := u; cur != v; cur = parent[cur] {
							cycle = append(cycle, cur)
						}
						return cycle
					}
				}
				if !found {
					state[u] = 2
					stack = stack[:len(stack)-1]
				}
			}
		}
	}
	return nil
}

func (e *Extractor) describeItemByIt(it extractedItem) string {
	if it.IsSupply {
		kind := "Supply"
		if it.FuncName != "" {
			kind += fmt.Sprintf(": argument '%s'", it.FuncName)
		} else if it.Expr != nil {
			var buf strings.Builder
			_ = printer.Fprint(&buf, it.Pkg.Fset, it.Expr)
			kind += ": " + buf.String()
		} else {
			kind += ": <anonymous>"
		}
		desc := fmt.Sprintf("%s -> %s", kind, it.RetType)
		if it.Position != "" {
			desc += fmt.Sprintf(" at %s", it.Position)
		}
		if it.InstanceName != "" {
			desc += fmt.Sprintf(" (name: %q)", it.InstanceName)
		}
		return desc
	}
	var kind string
	if it.IsInvoke {
		kind = "Invoke"
	} else {
		kind = "Provide"
	}
	funcName := model.FullFuncName(it.Pkg.PkgPath, it.FuncName)
	if it.IsClosure {
		funcName = it.FuncName + " (closure)"
	}
	desc := fmt.Sprintf("%s: %s", kind, funcName)
	if it.RetType != "" {
		desc += fmt.Sprintf(" -> %s", it.RetType)
	}
	if it.Position != "" {
		desc += fmt.Sprintf(" at %s", it.Position)
	}
	if it.InstanceName != "" {
		desc += fmt.Sprintf(" (name: %q)", it.InstanceName)
	}
	return desc
}

func (e *Extractor) describeItem(idx int) string {
	if idx < 0 || idx >= len(e.items) {
		return fmt.Sprintf("invalid index %d", idx)
	}
	return e.describeItemByIt(e.items[idx])
}

func (e *Extractor) formatCycleError(cycle []int) error {
	cycleDesc := functional.Map(cycle, e.describeItem)
	cycleInfo := strings.Join(cycleDesc, "\n    -> ")
	return fmt.Errorf("circular dependency detected:\n    %s\n  💡 Fix: break the cycle by removing or restructuring one of the dependencies", cycleInfo)
}

func (e *Extractor) computeOrder(adj [][]int, indeg []int) ([]int, error) {
	n := len(adj)
	indegCopy := make([]int, n)
	copy(indegCopy, indeg)

	order, err := topologicalSort(n, adj, indegCopy)
	if err != nil {
		cycle := e.findCycle(adj)
		return nil, e.formatCycleError(cycle)
	}
	return order, nil
}
