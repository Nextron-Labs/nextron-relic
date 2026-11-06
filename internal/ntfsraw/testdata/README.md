# Test data

`test.ntfs.dd.gz` is a gzip-compressed copy of `parser/test_data/test.ntfs.dd`
from [Velocidex go-ntfs](https://github.com/Velocidex/go-ntfs) v0.2.1
(SHA-256 of the uncompressed image:
`2b045901147e049e9a97f2e4afe47f4fa5d46c545e3a1c529a5b31d2835cf53a`).

go-ntfs is licensed under the Apache License 2.0. The image is used only by
the unit tests of `internal/ntfsraw` and is not part of the Relic binary.
