## What and why

<!-- One or two sentences. Link the issue (Fixes #123). -->

## How I tested it

<!-- macOS version and chip, which keyboard layouts were enabled, what you ran or typed (invented words, never real text). For UI changes: screenshots in English, Russian and Kazakh, light and dark. -->

## Checklist

- [ ] `swift test` passes **with the ABC, Russian and Kazakh layouts enabled** (check the summary: tests are skipped, not failed, without them)
- [ ] If I touched `windows/`: `cd windows && go test ./...` passes; if I changed the Swift algorithm, I ported it to Go and regenerated `windows/testdata/golden` with `windows/parity/regen-golden.sh`
- [ ] New or changed UI text is supplied in English, Russian and Kazakh
- [ ] Nothing is kept, written, logged or sent that `docs/PRIVACY.md` does not allow; if the privacy behaviour changed, the doc and its tests are updated in this pull request
- [ ] No new dependency, no network code, no secrets, personal data or real typed text in the diff
- [ ] Accuracy-relevant change: `CorpusTests` report before and after is attached
- [ ] Docs updated if behaviour, flags or numbers changed
