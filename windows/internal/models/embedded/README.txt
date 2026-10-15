This directory holds copies of ../../../../Resources/Models/*.bin for //go:embed.

They are generated (gitignored), never edited by hand:

    go generate ./...        (from windows/)
    ./build.sh               (macOS / Linux)
    .\build.ps1              (Windows)

Resources/Models is the single source of truth. TestEmbeddedModelsMatchSource fails when
a copy is missing or differs from it.
