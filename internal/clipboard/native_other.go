//go:build !windows

package clipboard

import "errors"

func nativeCopy(string) error { return errors.ErrUnsupported }
