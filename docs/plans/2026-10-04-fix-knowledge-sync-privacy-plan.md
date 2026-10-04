# Document the knowledge sync export destination

## Scope

Check the notice against knowledge.go, export.go and README; document opt-in behavior, both transports, data scope, and deletion boundaries. No runtime behavior changes.

## Validation

Run the relevant behavioral regression tests, the Go suite where engine code changes, and build/vet/format checks. For layout, use rendered runtime geometry and visual inspection; source-string checks are not evidence of fit.
