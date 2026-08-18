# dig Three-Way Comparison

> "Comparison" submodule of `system_prompt_dig_en.md`. This matrix is the single source of truth; the main docs and core skill no longer duplicate it.

| Feature | dig | Google Wire | Uber Fx |
|---------|-----|-------------|---------|
| Approach | Code generation | Code generation | Runtime reflection |
| Zero reflection / zero runtime dep | ✅ / ✅ | ✅ / ✅ | ❌ / ❌ |
| Direct value injection | ✅ `dig.Supply` (any expr) | ⚠️ `wire.Value` (constants only) | ✅ `fx.Supply` |
| Built-in Invoke | ✅ | ❌ | ✅ |
| Module nesting | ✅ explicit | ⚠️ flat composition | ✅ named |
| Interface binding | identity closure (inlined to conversion) | ✅ `wire.Bind` | ✅ `fx.As` |
| Generic support | ✅ compile-time (explicit instantiation) | ❌ | ⚠️ instantiated only |
| Multiple instances of same type | ✅ named parameters | ❌ needs wrapper types | ✅ named + value groups |
| Cleanup functions / lifecycle hooks | ❌ / ❌ | ✅ / ❌ | ✅ / ✅ |
| Decorators / optional deps | ❌ / ❌ | ❌ / ❌ | ✅ / ✅ |
| Error source location + fix suggestions | ✅ `file:line:col` + `💡 Fix:` | ⚠️ name only | ⚠️ runtime stack |
| Dependency graph visualization | ✅ `digen graph` (Mermaid) | ❌ | ✅ `fx.DotGraph` + `fx.VisualizeError` (DOT) |
| Resolution path explanation | ✅ `digen explain <type>` | ❌ | ❌ |
| Validation without running | ✅ `digen check` / generation | ✅ generation = validation | ✅ `fx.ValidateApp` |
| Maintenance status | ✅ active | ⚠️ archived (v0.7.0) | ✅ active |

**dig trade-offs**: deliberately minimal — no lifecycle hooks, no cleanup functions, no decorators, no optional dependencies, no app object/signal handling. `InitApp()` returns a bare `func(context.Context) error`; graceful shutdown is the caller's responsibility. In exchange: zero runtime overhead, compile-time safety, native generics, smallest API surface.

Note: `shanjunmei/dig` is unrelated to Uber's runtime reflection container `go.uber.org/dig`; do not confuse the two.
