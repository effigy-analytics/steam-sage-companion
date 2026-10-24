# alpha.2 release preparation

Source target: `0.1.0-alpha.2`; snapshot protocol remains 1. The guided installer
starts the installed background copy, opens Sage, and returns. Explicit Connect
opens exact-request local approval; browser differences, reopen failures,
decline/cancel races, first-read recovery, and alpha.1 fallback preserve choices.
Start at login remains optional and off by default. Native Windows installation
and browser integration are unverified and unsupported.

## Source preparation

- [x] Linux CI runs lifecycle tests and race tests through
  `scripts/with-isolated-network.sh`; Windows keeps unit/vet/build coverage.
- [x] Version/default packaging instructions target alpha.2.
- [x] Release builder requires clean source and creates deterministic allowlisted
  archives with source reference and Go version in `release.json`.
- [x] Installation, protocol, and privacy documents describe guided behavior.

## External delivery — controller owned, pending

- [ ] Fresh whole-change review across helper and Sage; address findings.
- [ ] Green public Linux and Windows checks, helper PR merge.
- [ ] Build clean archives from that verified public merge commit, compare repeat
  hashes, and scan source/archive contents for secrets.
- [ ] Create source archive with `git archive` from the verified public commit.
- [ ] Publish Linux/source/checksums only. Do not publish the Windows zip.
- [ ] Anonymously download and verify public checksums, archive, and installer.
- [ ] Update Sage's alpha.1 pin only after public alpha.2 asset verification.
- [ ] Green Sage checks and merge; verify dev/prod, live download and connected
  setup with authoritative first-read completion.

Local candidate hashes and browser results are recorded in Sage's dated guided
setup validation document and the Task 6 report. Local builds do not establish a
published release, native Windows support, Firefox coverage, or deployment.
