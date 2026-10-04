# Fit Settings and onboarding without scrolling

## Scope

Use grouped columns with explicit pagination for unbounded lists and long text. Retain all controls, consent choices and complete action results. Verify native QML at host size and smaller screens in normal and error states.

## Validation

Run the relevant behavioral regression tests, the Go suite where engine code changes, and build/vet/format checks. For layout, use rendered runtime geometry and visual inspection; source-string checks are not evidence of fit.

## Delivered

Settings groups provider/privacy, capture/categories and prompts into three columns. Categories, presets and providers have selectors; complete long text and errors use measured pages. Onboarding exposes each guided step without a clipped Flickable. Settings opens at 1180 pixels wide. Other tabs retain their existing layouts.

`python tests/ui/verify-fit.py` passed 36 runtime scenarios: dark/light palettes, all five onboarding steps and Settings at logical 1280×720, 1280×800 and 1920×1080 viewports. The 1280-wide cases use the actual 1160-pixel inner Settings width and reserve 180 pixels for popup chrome. Settings height was 535 pixels against a 540-pixel budget at 720 pixels. Fixtures include 120 categories, 20 providers, long prompts and multiline errors. Page roundtrip/Unicode and editing preserve surrounding text. Host interfaces are inert fixtures, so this is component/runtime evidence, not a full host popup acceptance test.
