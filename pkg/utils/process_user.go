package utils

import "errors"

var ErrProcessUserTokenUnavailable = errors.New("process user token unavailable")

func IsProcessUserTokenUnavailable(err error) bool {
	return errors.Is(err, ErrProcessUserTokenUnavailable)
}
