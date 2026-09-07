//go:build !linux && !darwin

package sprint

import "fmt"

func qaStorageAvailable(string) (int64, error) {
	return 0, fmt.Errorf("QA storage measurement is unsupported on this host")
}
func qaProcessAlive(int) bool { return true }

func qaStorageTryLock(string) (func(), bool, error) {
	return nil, false, fmt.Errorf("QA resource reservations are unsupported on this host")
}
