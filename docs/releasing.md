# Releasing Work Ledger

This project publishes source releases. Users install a tagged version with Go:

```sh
go install github.com/NelsonBrandao/work-ledger/cmd/work-ledger@vX.Y.Z
```

Releases are source-only: there are no prebuilt binaries or checksums yet.

## Checklist

1. Start from an up-to-date, clean `main` branch.
2. Run the release checks:

   ```sh
   go test ./...
   go vet ./...
   go test -race ./...
   go build -o /private/tmp/work-ledger ./cmd/work-ledger
   ```

3. Update user documentation and the release notes as needed, then commit and
   push the changes to `main`.
4. Create and push an annotated semantic-version tag:

   ```sh
   git tag -a vX.Y.Z -m "Release vX.Y.Z"
   git push origin vX.Y.Z
   ```

5. Create the GitHub release from that tag:

   ```sh
   gh release create vX.Y.Z --repo NelsonBrandao/work-ledger \
     --title "vX.Y.Z" --generate-notes
   ```

6. Verify both source installation and the release page:

   ```sh
   go install github.com/NelsonBrandao/work-ledger/cmd/work-ledger@vX.Y.Z
   work-ledger help
   gh release view vX.Y.Z --repo NelsonBrandao/work-ledger
   ```

Use `go install ...@vX.Y.Z` in user documentation rather than a branch name so
installations are reproducible. A future binary-release process should add
platform builds, checksums, and signing before it advertises downloadable
artifacts.
