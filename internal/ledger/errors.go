package ledger

import "errors"

// ErrNotFound is returned when a workitem or card lookup by id finds no
// matching file.
var ErrNotFound = errors.New("ledger: not found")
