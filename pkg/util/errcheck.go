package util

import "io"

// CheckClose is used to check the return from Close in a defer statement.
// From https://groups.google.com/d/msg/golang-nuts/-eo7navkp10/BY3ym_vMhRcJ
func CheckClose(c io.Closer) {
	err := c.Close()
	if err != nil {
		Error("Failed to close deferred io.Closer, err: %v", err)
	}
}
