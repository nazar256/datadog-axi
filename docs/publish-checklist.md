# Publish checklist

## In-repo readiness

- [x] README explains positioning, install, auth, discovery, and examples
- [x] install docs exist for release installer, manual release assets, and local build
- [x] AI-agent usage doc exists
- [x] Investigation guides and documentation inventory exist
- [x] license, contributing, and security docs exist
- [x] CI workflow runs tests and build checks
- [x] release workflow builds release archives and uploads checksums

## Manual GitHub follow-up

- [ ] Set repository description, homepage, and topics from `docs/github-metadata.md`
- [ ] Upload a social preview image aligned with the README positioning
- [ ] Review the first release notes so they explain why this repo exists
- [ ] Verify release assets are attached for Linux amd64/arm64 and macOS amd64/arm64
- [ ] Publish the draft release so `/releases/latest` resolves
- [ ] Confirm the release installer works from a clean Linux shell
- [ ] Confirm README snippets still match the latest tagged release version

## Nice follow-ups for later

- [ ] Add a short terminal demo GIF or screenshot to the README if it stays lightweight
- [ ] Consider signed or provenance-backed release artifacts if public adoption grows
