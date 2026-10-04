# Apply JSON settings patches to the active provider

## Scope

Reproduce legacy-only and panel-snapshot patches, preserve unrelated providers and explicit provider edits, and prevent Settings from resending stale advanced provider fields.

## Validation

Run the relevant behavioral regression tests, the Go suite where engine code changes, and build/vet/format checks. For layout, use rendered runtime geometry and visual inspection; source-string checks are not evidence of fit.
