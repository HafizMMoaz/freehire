## 1. Three-state evidence in the board report

- [x] 1.1 Add a failing test: a board whose only posting has no `is_tech` verdict must not be listed, while a board whose postings were classified non-technical still is, and a technical verdict or a tagged skill still keeps a board off the list
- [x] 1.2 Add a failing test that the report states how many boards it withheld
- [x] 1.3 Replace the boolean evidence map with a `boardVerdict{technical, determined}`, listing a board only when `determined && !technical`
- [x] 1.4 Print the withheld count above the list, and reword the empty-list line so it no longer claims every listed board posted something technical
- [x] 1.5 Update the command's package documentation, which described the old rule

## 2. Verification

- [x] 2.1 `go build ./... && go vet ./... && gofmt -l` clean
- [x] 2.2 `go test ./cmd/... ./internal/...` green
- [ ] 2.3 Rebuild `prune` on prod and re-run `--boards`; confirm the withheld count lands near the measured 11023 of 17841 and the list near 6818

**Run on production 2026-07-31 — behaviour confirmed, magnitudes NOT.** The report prints
the withheld count above the list and the three-state rule holds, which is what 1.3/1.4
changed. But the numbers have moved well past "near": **withheld 12007** against the
measured 11023 (+9%), and **9828 boards listed** against 6818 (+44%). The pass scanned
5.4M rows and 77709 boards in 20m17s.

Left unticked deliberately: the task's acceptance is "lands near the measured", and +44%
is not near. Someone who knows whether the catalogue simply grew, or whether more boards
crossed into determined-non-technical, should read those two numbers before this closes.
