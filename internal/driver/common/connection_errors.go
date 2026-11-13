package common

import "errors"

// ErrTTIRENEG signals that connection establishment should fall back
// after the server requests TTC renegotiation.
var ErrTTIRENEG = errors.New("server requested TTC renegotiation")
