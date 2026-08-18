# dig Troubleshooting Skill

> "Troubleshooting" submodule of `system_prompt_dig_en.md`. Priority order and common error maps for generation/build failures.

## 1. Error Format

Every dig error carries its source location (`file:line:col`) and a `💡 Fix:` suggestion — no `-debug` needed to locate the problem.

## 2. Check Priority

When generation or build fails, verify in this order:

1. **Closure captures locals**: Provide/Invoke closures capture a local variable inside InitApp → use package-level variable or parameter injection
2. **Primitive type conflict**: multiple providers of the same primitive type without distinction → use a wrapper type (e.g. `type UseMySQL bool`)
3. **Duplicate Module**: the same Module registered more than once → de-duplicate or rename
4. **Uninstantiated generics**: wrote `dig.Provide(NewStore)` instead of `dig.Provide(NewStore[int])` → instantiate explicitly
5. **Ambiguous dependency**: multiple instances of a type exist but the consumer didn't name the parameter → consume via named parameter
6. **Cross-package unexported reference**: a lifted closure references an unexported cross-package symbol → export the symbol or implement it in-package

## 3. Common Error → Fix

| Symptom | Cause | Fix |
|---------|-------|-----|
| `private` / unexported symbol error | unexported cross-package call inside closure | export the symbol or implement in-package |
| ambiguous dependency error | same-type multi-instance not named | consume via named parameter (e.g. `mainDB`) |
| variable-shadowing warning | name clash in generated code | usually handled automatically by ShadowGuard; review naming manually |
| duplicate InitApp declaration | di.go built by `go build` without `//go:build digen` | add the build tag to di.go |
| provider declares a `context.Context` parameter | rejected at generation-time check | pass context inside Invoke instead |

## 4. Diagnostic Tools

- `digen check [pkgs]`: validate the DI contract only (extraction + unused-provider check), writes no file
- `digen graph [pkgs]`: Mermaid dependency graph, locate missing/cyclic edges
- `digen explain <type>`: explain how a type is resolved
- `-debug`: print alias mapping diagnostics for alias-related anomalies
