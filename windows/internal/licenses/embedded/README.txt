This directory holds copies of ../../../../LICENSE and ../../../../THIRD_PARTY_NOTICES.md for
//go:embed.

They are generated (gitignored), never edited by hand:

    go generate ./...        (from windows/)
    ./build.sh               (macOS / Linux)
    .\build.ps1              (Windows)

The files at the repository root are the single source of truth. TestEmbeddedLicensesMatchSource
fails when a copy is missing or differs from it.
