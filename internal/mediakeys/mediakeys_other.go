//go:build !windows

package mediakeys

import "errors"

// Listen is only implemented on Windows; Linux gets media keys via MPRIS.
func Listen(func(Key)) (stop func(), err error) {
	return func() {}, errors.ErrUnsupported
}
